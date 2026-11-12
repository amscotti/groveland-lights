package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Exit codes for guard, per plan §6.1.5.
const (
	exitProtected = 3
	exitOwnership = 4
	exitRatchet   = 5
	exitMarkers   = 6
	exitDeps      = 7
	exitHandoff   = 8
	exitDirty     = 9
)

func cmdGuard(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: harness guard T-###")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	pol, err := loadPolicy(root)
	if err != nil {
		return err
	}
	g, err := loadGraph(root)
	if err != nil {
		return err
	}
	t := g.task(args[0])
	if t == nil {
		return fmt.Errorf("unknown task %s", args[0])
	}
	wt := filepath.Join(filepath.Dir(root), "wt", t.ID)
	if _, err := os.Stat(wt); err != nil {
		return fmt.Errorf("worktree %s not found", wt)
	}
	os.Exit(guardTask(root, wt, t, pol))
	return nil
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	var out, errB bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errB
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %v: %w: %s", args, err, strings.TrimSpace(errB.String()))
	}
	return out.String(), nil
}

func gitOut2(dir string, args ...string) string {
	out, err := gitOut(dir, args...)
	if err != nil {
		return ""
	}
	return out
}

func repoRoot() (string, error) {
	out, err := gitOut("", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// guardTask runs the nine checks and returns the exit code of the FIRST failing
// check in plan order (0 = clean). Every failure prints an ERROR: line.
func guardTask(root, wt string, t *Task, pol Policy) int {
	changed := gitChanged(wt)

	if code, ok := checkProtectedPaths(changed, t, pol); !ok {
		return code
	}
	if err := VerifyManifest(wt); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return exitProtected
	}
	if code, ok := checkOwnership(changed, t); !ok {
		return code
	}
	if code, ok := checkRatchet(root, wt); !ok {
		return code
	}
	if code, ok := checkMarkers(wt); !ok {
		return code
	}
	if code, ok := checkDeps(changed, wt, t); !ok {
		return code
	}
	if code, ok := checkHandoff(wt, t); !ok {
		return code
	}
	// 8. Special-casing heuristic: warning only, surfaced to reviewer-alt.
	for _, h := range specialCaseHeuristic(ParseUnifiedAdded(gitOut2(wt, "diff", "main...HEAD")), wt) {
		fmt.Fprintf(os.Stderr, "WARN: possible fixture special-casing: %s\n", h)
	}
	if code, ok := checkCleanWorktree(wt); !ok {
		return code
	}
	return 0
}

// checkProtectedPaths is check 1.
func checkProtectedPaths(changed []string, t *Task, pol Policy) (int, bool) {
	for _, p := range changed {
		if pat, hit := protectedMatch(pol.Protected, p); hit {
			if t.Type == "acceptance" && acceptanceAllowed(p) {
				continue
			}
			fmt.Fprintf(os.Stderr, "ERROR: protected path %s (matches %s)\n", p, pat)
			return exitProtected, false
		}
	}
	return 0, true
}

// checkOwnership is check 3.
func checkOwnership(changed []string, t *Task) (int, bool) {
	allowed := append(append([]string{}, t.Owns...), "harness/handoffs/"+t.ID+".md")
	for _, p := range changed {
		if !ownsCovered(allowed, p) {
			fmt.Fprintf(os.Stderr, "ERROR: out of ownership: %s not covered by owns of %s\n", p, t.ID)
			return exitOwnership, false
		}
	}
	return 0, true
}

// checkRatchet is check 4 (counts + new skip markers).
func checkRatchet(root, wt string) (int, bool) {
	if base := readBaseline(root); base != nil {
		goBr, pwBr := countGoTests(wt), countPwTests(wt)
		if err := RatchetCompare(base.Go, goBr, base.Pw, pwBr); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: ratchet: %v\n", err)
			return exitRatchet, false
		}
	} else {
		fmt.Fprintln(os.Stderr, "WARN: no ratchet baseline; run `harness protect` once on main")
	}
	for _, a := range ParseUnifiedAdded(gitOut2(wt, "diff", "main...HEAD")) {
		if strings.HasPrefix(a.path, "acceptance/") {
			continue
		}
		for _, m := range []string{"t.Skip(", "test.skip(", "test.only(", ".fixme("} {
			if strings.Contains(a.line, m) {
				fmt.Fprintf(os.Stderr, "ERROR: ratchet: new %q at %s:%d\n", m, a.path, a.n)
				return exitRatchet, false
			}
		}
	}
	return 0, true
}

// checkMarkers is check 5.
func checkMarkers(wt string) (int, bool) {
	for _, f := range ScanAddedLines(ParseUnifiedAdded(gitOut2(wt, "diff", "main...HEAD"))) {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", f)
		return exitMarkers, false
	}
	return 0, true
}

// checkDeps is check 6.
func checkDeps(changed []string, wt string, t *Task) (int, bool) {
	if !depsChanged(changed) {
		return 0, true
	}
	for _, d := range DiffDeps(gitShow(wt, "main:go.mod"), readFileOr(filepath.Join(wt, "go.mod")), "go") {
		if strings.HasPrefix(d, "golang.org/x/vuln") || strings.HasPrefix(d, "(") {
			continue
		}
		if !DepAllowed(d, t.AllowDeps) {
			fmt.Fprintf(os.Stderr, "ERROR: dependency %q not in allow_deps of %s\n", d, t.ID)
			return exitDeps, false
		}
	}
	for _, d := range DiffDeps(gitShow(wt, "main:package.json"), readFileOr(filepath.Join(wt, "package.json")), "npm") {
		if !DepAllowed(d, t.AllowDeps) {
			fmt.Fprintf(os.Stderr, "ERROR: dependency %q not in allow_deps of %s\n", d, t.ID)
			return exitDeps, false
		}
	}
	return 0, true
}

// checkHandoff is check 7.
func checkHandoff(wt string, t *Task) (int, bool) {
	if _, err := os.Stat(filepath.Join(wt, "harness", "handoffs", t.ID+".md")); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: missing handoff harness/handoffs/%s.md\n", t.ID)
		return exitHandoff, false
	}
	if strings.TrimSpace(gitOut2(wt, "rev-list", "--count", "main..HEAD")) == "0" {
		fmt.Fprintf(os.Stderr, "ERROR: no commits on task/%s\n", t.ID)
		return exitHandoff, false
	}
	return 0, true
}

// checkCleanWorktree is check 9 (E3).
func checkCleanWorktree(wt string) (int, bool) {
	dirty := DirtyLines(gitOut2(wt, "status", "--porcelain"))
	for _, d := range dirty {
		fmt.Fprintf(os.Stderr, "ERROR: dirty worktree: %s\n", d)
	}
	if len(dirty) > 0 {
		return exitDirty, false
	}
	return 0, true
}

func acceptanceAllowed(p string) bool {
	return matchGlob("acceptance/**", p) || p == "harness/protected.sha256" ||
		matchGlob("harness/tasks/**", p)
}

// specialCaseHeuristic: string literals in added non-test code that appear verbatim
// in acceptance fixtures (testdata/). Warning-level only.
func specialCaseHeuristic(added []addedLine, wt string) []string {
	var out []string
	for _, a := range added {
		if strings.HasSuffix(a.path, "_test.go") || strings.HasPrefix(a.path, "acceptance/") {
			continue
		}
		for _, lit := range stringLiterals(a.line) {
			if len(lit) < 8 { // too short to be meaningful
				continue
			}
			if fixtureContains(wt, lit) {
				out = append(out, fmt.Sprintf("%s:%d: %q appears in testdata fixtures", a.path, a.n, lit))
			}
		}
	}
	return out
}

func stringLiterals(line string) []string {
	var out []string
	var cur []rune
	inQ := false
	for _, r := range line {
		switch {
		case r == '"' && inQ:
			out = append(out, string(cur))
			cur, inQ = nil, false
		case r == '"':
			inQ = true
		case inQ:
			cur = append(cur, r)
		}
	}
	return out
}

func fixtureContains(wt, lit string) bool {
	// Cheap grep over testdata text fixtures only; images ignored.
	cmd := exec.CommandContext(context.Background(), "grep", "-rF", lit, filepath.Join(wt, "testdata"))
	return cmd.Run() == nil
}

func gitChanged(wt string) []string {
	var files []string
	for _, l := range strings.Split(gitOut2(wt, "diff", "--name-only", "main...HEAD"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files
}

func gitShow(wt, ref string) string {
	return gitOut2(wt, "show", ref)
}

func readFileOr(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func depsChanged(changed []string) bool {
	for _, p := range changed {
		if p == "go.mod" || p == "go.sum" || p == "package.json" || p == "package-lock.json" {
			return true
		}
	}
	return false
}

// baseline for the ratchet, stored at harness/.ratchet.json by protect/sched.
type baseline struct {
	Go int `json:"go"`
	Pw int `json:"pw"`
}

func readBaseline(root string) *baseline {
	b, err := os.ReadFile(filepath.Join(root, "harness", ".ratchet.json"))
	if err != nil {
		return nil
	}
	var bl baseline
	if json.Unmarshal(b, &bl) != nil {
		return nil
	}
	return &bl
}

func countGoTests(dir string) int {
	cmd := exec.CommandContext(context.Background(), "go", "test", "-list", ".*", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-count=1")
	out, _ := cmd.CombinedOutput()
	n := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "Test") {
			n++
		}
	}
	return n
}

func countPwTests(dir string) int {
	cmd := exec.CommandContext(context.Background(), "npx", "playwright", "test", "--list")
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	n := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "›") { // playwright list bullet
			n++
		}
	}
	return n
}
