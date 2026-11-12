package main

import (
	"bytes"
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
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errB bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errB
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %v: %v: %s", args, err, strings.TrimSpace(errB.String()))
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

	// 1. Protected paths. Exception: acceptance tasks (test-author) may touch
	// acceptance/**, the manifest, their packet and the graph.
	for _, p := range changed {
		if pat, hit := protectedMatch(pol.Protected, p); hit {
			if t.Type == "acceptance" && acceptanceAllowed(p) {
				continue
			}
			if t.Type == "infra" && !matchGlob("acceptance/**", p) && !matchGlob(".opencode/**", p) && !matchGlob("cmd/harness/**", p) && !matchGlob("harness/**", p) {
				// infra tasks may touch build/deploy files but not the above
			} else if t.Type != "infra" {
				fmt.Fprintf(os.Stderr, "ERROR: protected path %s (matches %s)\n", p, pat)
				return exitProtected
			}
		}
	}

	// 2. Checksum manifest.
	if err := VerifyManifest(wt); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return exitProtected
	}

	// 3. Ownership: every changed file must be covered by owns + the handoff.
	allowed := append(append([]string{}, t.Owns...), "harness/handoffs/"+t.ID+".md")
	for _, p := range changed {
		if !ownsCovered(allowed, p) {
			fmt.Fprintf(os.Stderr, "ERROR: out of ownership: %s not covered by owns of %s\n", p, t.ID)
			return exitOwnership
		}
	}

	// 4. Test ratchet: counts vs the baseline captured on main.
	if base := readBaseline(root); base != nil {
		goBr, pwBr := countGoTests(wt), countPwTests(wt)
		if err := RatchetCompare(base.Go, goBr, base.Pw, pwBr); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: ratchet: %v\n", err)
			return exitRatchet
		}
	} else {
		fmt.Fprintln(os.Stderr, "WARN: no ratchet baseline; run `harness protect` once on main")
	}

	// 4b. New skip markers in added lines.
	diff := gitOut2(wt, "diff", "main...HEAD")
	added := ParseUnifiedAdded(diff)
	for _, a := range added {
		if strings.HasPrefix(a.path, "acceptance/") {
			continue
		}
		for _, m := range []string{"t.Skip(", "test.skip(", "test.only(", ".fixme("} {
			if strings.Contains(a.line, m) {
				fmt.Fprintf(os.Stderr, "ERROR: ratchet: new %q at %s:%d\n", m, a.path, a.n)
				return exitRatchet
			}
		}
	}

	// 5. Forbidden markers in added lines.
	for _, f := range ScanAddedLines(added) {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", f)
		return exitMarkers
	}

	// 6. Dependency additions outside allow_deps.
	if depsChanged(changed) {
		for _, d := range DiffDeps(gitShow(wt, "main:go.mod"), readFileOr(filepath.Join(wt, "go.mod")), "go") {
			if strings.HasPrefix(d, "golang.org/x/vuln") || strings.HasPrefix(d, "(") {
				continue
			}
			if !DepAllowed(d, t.AllowDeps) {
				fmt.Fprintf(os.Stderr, "ERROR: dependency %q not in allow_deps of %s\n", d, t.ID)
				return exitDeps
			}
		}
		for _, d := range DiffDeps(gitShow(wt, "main:package.json"), readFileOr(filepath.Join(wt, "package.json")), "npm") {
			if !DepAllowed(d, t.AllowDeps) {
				fmt.Fprintf(os.Stderr, "ERROR: dependency %q not in allow_deps of %s\n", d, t.ID)
				return exitDeps
			}
		}
	}

	// 7. Handoff exists and at least one commit on the branch.
	if _, err := os.Stat(filepath.Join(wt, "harness", "handoffs", t.ID+".md")); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: missing handoff harness/handoffs/%s.md\n", t.ID)
		return exitHandoff
	}
	if strings.TrimSpace(gitOut2(wt, "rev-list", "--count", "main..HEAD")) == "0" {
		fmt.Fprintf(os.Stderr, "ERROR: no commits on task/%s\n", t.ID)
		return exitHandoff
	}

	// 8. Special-casing heuristic: warning only, surfaced to reviewer-alt.
	if hits := specialCaseHeuristic(added, wt); len(hits) > 0 {
		for _, h := range hits {
			fmt.Fprintf(os.Stderr, "WARN: possible fixture special-casing: %s\n", h)
		}
	}

	// 9. Clean worktree (E3).
	if dirty := DirtyLines(gitOut2(wt, "status", "--porcelain")); len(dirty) > 0 {
		for _, d := range dirty {
			fmt.Fprintf(os.Stderr, "ERROR: dirty worktree: %s\n", d)
		}
		return exitDirty
	}

	return 0
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
	cmd := exec.Command("grep", "-rF", lit, filepath.Join(wt, "testdata"))
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
	cmd := exec.Command("go", "test", "-list", ".*", "./...")
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
	cmd := exec.Command("npx", "playwright", "test", "--list")
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
