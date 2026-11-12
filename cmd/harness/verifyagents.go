package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// agentInfo mirrors the relevant parts of `opencode debug agents` JSON output
// (verified machine-readable on OpenCode v2.2.7).
type agentInfo struct {
	ID          string           `json:"id"`
	Mode        string           `json:"mode"`
	Model       string           `json:"model,omitempty"`
	Permissions []permissionRule `json:"permissions"`
}

type permissionRule struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   string `json:"effect"`
}

// cmdVerifyAgents checks every agent file against policy: deny-all baseline, the
// trailing denies, vision routing, and model IDs.
func cmdVerifyAgents(args []string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	pol, err := loadPolicy(root)
	if err != nil {
		return err
	}

	infos := agentsFromDebug(root) // may be empty
	byID := map[string]agentInfo{}
	for _, a := range infos {
		byID[a.ID] = a
	}

	var roles []string
	for r := range pol.Roles {
		roles = append(roles, r)
	}
	sort.Strings(roles)

	fmt.Printf("verify-agents: %d roles in policy\n", len(roles))
	failed := false
	for _, role := range roles {
		if err := verifyOneAgent(root, role, pol, byID); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: agent %s: %v\n", role, err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("verify-agents: FAILED")
	}
	fmt.Println("verify-agents: all agents conform to policy")
	return nil
}

// verifyOneAgent checks one role; nil means conforming.
func verifyOneAgent(root, role string, pol Policy, byID map[string]agentInfo) error {
	wantModelLogical := pol.Roles[role]
	wantModelID := pol.Models[wantModelLogical]

	info, live := byID[role]
	fm, err := parseAgentFile(filepath.Join(root, ".opencode", "agents", role+".md"))
	if err != nil {
		return err
	}

	rules := fm.Permissions
	if live {
		rules = info.Permissions
	}
	if len(rules) == 0 {
		return fmt.Errorf("no permission rules found")
	}
	if err := checkPermissionBaselines(rules, role, pol); err != nil {
		return err
	}
	if fm.Model != wantModelID {
		return fmt.Errorf("model %q != policy %q", fm.Model, wantModelID)
	}
	if live && info.Model != "" && info.Model != wantModelID {
		return fmt.Errorf("live model %q != policy %q", info.Model, wantModelID)
	}
	if contains(pol.VisionRoles, role) && !contains(pol.VisionModels, wantModelLogical) {
		return fmt.Errorf("vision role on non-vision model %s", wantModelLogical)
	}

	src := "file"
	if live {
		src = "live"
	}
	vision := ""
	if contains(pol.VisionRoles, role) {
		vision = " [vision]"
	}
	fmt.Printf("  %-16s model=%s (%s)%s ok\n", role, wantModelID, src, vision)
	return nil
}

// checkPermissionBaseline enforces the deny-all baseline and the trailing denies.
func checkPermissionBaselines(rules []permissionRule, role string, pol Policy) error {
	// Deny-all baseline: last match for (*,*) must be deny.
	if eff := lastMatch(rules, "*", "*"); eff != "deny" {
		return fmt.Errorf("deny-all baseline not effective (last match for */* is %q)", eff)
	}
	// Trailing denies for all agents.
	for _, action := range []string{"subagent", "execute", "webfetch", "websearch", "question"} {
		if eff := lastMatch(rules, action, "*"); eff != "deny" {
			return fmt.Errorf("%s not effectively denied", action)
		}
	}
	// external_directory: denied for all except test-author (scoped allow).
	if eff := lastMatch(rules, "external_directory", "*"); eff != "deny" {
		if role != "test-author" {
			return fmt.Errorf("external_directory not denied")
		}
	}
	if role == "test-author" {
		if eff := lastMatch(rules, "external_directory", pol.HoldoutDir+"/*"); eff != "allow" {
			return fmt.Errorf("holdout dir not allowed")
		}
	}
	return nil
}

// lastMatch applies OpenCode semantics: rules in order, last matching rule wins.
func lastMatch(rules []permissionRule, action, resource string) string {
	eff := ""
	for _, r := range rules {
		if matchSimple(r.Action, action) && matchSimple(r.Resource, resource) {
			eff = r.Effect
		}
	}
	return eff
}

func matchSimple(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func agentsFromDebug(root string) []agentInfo {
	dir, err := repoWorkingDir(root)
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(context.Background(), "opencode", "debug", "agents")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil
	}
	var agents []agentInfo
	if err := json.Unmarshal(out.Bytes(), &agents); err != nil {
		return nil
	}
	return agents
}

func repoWorkingDir(root string) (string, error) {
	if filepath.IsAbs(root) {
		return root, nil
	}
	return os.Getwd()
}

// agentFrontmatter is the minimal parsed frontmatter of an agent .md file.
type agentFrontmatter struct {
	Model       string
	Mode        string
	Permissions []permissionRule
}

var reFmModel = regexp.MustCompile(`(?m)^model:\s*(\S+)`)
var reFmMode = regexp.MustCompile(`(?m)^mode:\s*(\S+)`)
var reFmRule = regexp.MustCompile(`(?m)^\s*-\s*\{\s*action:\s*"([^"]*)"\s*,\s*resource:\s*"([^"]*)"\s*,\s*effect:\s*"([^"]*)"\s*\}`)

func parseAgentFile(p string) (agentFrontmatter, error) {
	var fm agentFrontmatter
	b, err := os.ReadFile(p)
	if err != nil {
		return fm, err
	}
	text := string(b)
	end := strings.Index(text, "\n---")
	if end > 0 {
		text = text[:end]
	}
	if m := reFmModel.FindStringSubmatch(text); m != nil {
		fm.Model = m[1]
	}
	if m := reFmMode.FindStringSubmatch(text); m != nil {
		fm.Mode = m[1]
	}
	for _, m := range reFmRule.FindAllStringSubmatch(text, -1) {
		fm.Permissions = append(fm.Permissions, permissionRule{Action: m[1], Resource: m[2], Effect: m[3]})
	}
	if fm.Model == "" || len(fm.Permissions) == 0 {
		return fm, fmt.Errorf("frontmatter incomplete (model or permissions missing)")
	}
	return fm, nil
}
