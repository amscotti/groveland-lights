package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdSelftest runs the harness self-test (plan §6.2 step 12, §6.3 rows 3-4).
//
// Deterministic scenarios (no LLM): S2 tamper-spec, S3 out-of-ownership, S4
// deleted-test ratchet, S5 red-check gate refusal, S7 owns-overlap scheduling,
// S9 dirty worktree. Live scenarios (real agents): S1 happy path, S6 permission
// probes, S8 holdout isolation.
func cmdSelftest(args []string) error {
	live := false
	for _, a := range args {
		if a == "--live" {
			live = true
		}
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}

	passed, failed := 0, 0
	report := func(name string, ok bool, detail string) {
		if ok {
			passed++
			fmt.Printf("  PASS %s: %s\n", name, detail)
		} else {
			failed++
			fmt.Printf("  FAIL %s: %s\n", name, detail)
		}
	}

	// Throwaway clone under TMPDIR.
	tmp, err := os.MkdirTemp("", "gl-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	clone := filepath.Join(tmp, "repo")
	if out, err := gitOut("", "clone", root, clone); err != nil {
		return fmt.Errorf("clone: %v: %s", err, out)
	}
	linkNodeModules(root, clone)
	// Never let the selftest push to the real repo: origin becomes a local bare.
	bare := filepath.Join(tmp, "origin.git")
	if out, err := gitOut("", "init", "--bare", bare); err != nil {
		return fmt.Errorf("bare: %v: %s", err, out)
	}
	if out, err := gitOut(clone, "remote", "set-url", "origin", bare); err != nil {
		return fmt.Errorf("remote: %v: %s", err, out)
	}

	if err := selftestFixture(clone); err != nil {
		return fmt.Errorf("fixture: %w", err)
	}

	pol, _ := loadPolicy(clone)

	type scenario struct {
		name string
		run  func() (bool, string)
	}
	scenarios := []scenario{
		{"S2 tamper-spec", func() (bool, string) { return scTamper(clone, pol) }},
		{"S3 out-of-ownership", func() (bool, string) { return scOwnership(clone, pol) }},
		{"S4 deleted-test", func() (bool, string) { return scDeletedTest(clone, pol) }},
		{"S5 red-check", func() (bool, string) { return scRedCheck(clone, pol) }},
		{"S7 overlap", func() (bool, string) { return scOverlap(clone, pol) }},
		{"S9 dirty-worktree", func() (bool, string) { return scDirty(clone, pol) }},
	}
	fmt.Println("selftest: deterministic scenarios")
	for _, s := range scenarios {
		ok, detail := s.run()
		report(s.name, ok, detail)
	}

	if live {
		fmt.Println("selftest: live scenarios (real agents)")
		ok, detail := scLiveHappy(clone, pol)
		report("S1 happy path", ok, detail)
		ok, detail = scLiveProbes(root, clone, pol)
		report("S6 permission probes", ok, detail)
		ok, detail = scLiveHoldout(clone, pol)
		report("S8 holdout isolation", ok, detail)
	}

	fmt.Printf("selftest: %d/%d deterministic scenarios passed\n", passed, 6)
	if failed > 0 {
		return fmt.Errorf("selftest FAILED (%d failures)", failed)
	}
	return nil
}

// selftestFixture writes the scripted test-author output (A-900 spec), the fixture
// graph with scenario tasks, and commits everything on the clone's main.
func selftestFixture(clone string) error {
	// Scripted test-author: acceptance spec for /version (A-900).
	spec := `import { test, expect } from '@playwright/test';

test('@A-900 @smoke /version returns JSON', async ({ request }) => {
  const res = await request.get('/version');
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(typeof body.version).toBe('string');
  expect(typeof body.commit).toBe('string');
});
`
	if err := os.MkdirAll(filepath.Join(clone, "acceptance", "e2e"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(clone, "acceptance", "e2e", "version.spec.ts"), []byte(spec), 0o644); err != nil {
		return err
	}

	// Contract entry.
	contractPath := filepath.Join(clone, "acceptance", "contract.json")
	var contract struct {
		Assertions []Assertion `json:"assertions"`
	}
	if b, err := os.ReadFile(contractPath); err == nil {
		_ = json.Unmarshal(b, &contract)
	}
	gateTrue := true
	contract.Assertions = append(contract.Assertions, Assertion{
		ID: "A-900", When: "GET /version", Shall: "return 200 JSON {version, commit}", Gate: &gateTrue,
	})
	cb, _ := json.MarshalIndent(contract, "", "  ")
	if err := os.WriteFile(contractPath, append(cb, '\n'), 0o644); err != nil {
		return err
	}

	// A deletable passing test on main (for S4).
	if err := os.MkdirAll(filepath.Join(clone, "internal", "web"), 0o755); err != nil {
		return err
	}
	dummy := `package web

import "testing"

func TestSelftestDummyAlwaysPasses(t *testing.T) {
	t.Log("fixture test for the ratchet scenario")
}
`
	if err := os.WriteFile(filepath.Join(clone, "internal", "web", "dummy_fixture_test.go"), []byte(dummy), 0o644); err != nil {
		return err
	}

	// Task packets + graph.
	tasks := []Task{
		{ID: "T-000A", Type: "acceptance", Role: "test-author", Title: "Acceptance for /version (scripted)", Owns: []string{"acceptance/**"}, Claims: []string{"A-900", "A-000"}, Status: StatusDone, Budget: Budget{Steps: 50, WallMin: 15}},
		{ID: "T-000", Type: "feature", Role: "worker", Title: "GET /version returns JSON {version, commit}", Deps: []string{"T-000A"}, Owns: []string{"internal/web/version*.go"}, Claims: []string{"A-900"}, Status: StatusReady, Budget: Budget{Steps: 120, WallMin: 30}},
		{ID: "T-S3", Type: "chore", Role: "worker", Title: "ownership scenario", Owns: []string{"docs/nothing/**"}, Status: StatusReady, Budget: Budget{Steps: 10, WallMin: 5}},
		{ID: "T-S4", Type: "chore", Role: "worker", Title: "ratchet scenario", Owns: []string{"internal/**"}, Status: StatusReady, Budget: Budget{Steps: 10, WallMin: 5}},
		{ID: "T-S5", Type: "feature", Role: "worker", Title: "red check scenario", Deps: []string{"T-000A"}, Owns: []string{"internal/**"}, Claims: []string{"A-900"}, Status: StatusReady, Budget: Budget{Steps: 10, WallMin: 5}},
		{ID: "T-S9", Type: "chore", Role: "worker", Title: "dirty worktree scenario", Owns: []string{"internal/**"}, Status: StatusReady, Budget: Budget{Steps: 10, WallMin: 5}},
		{ID: "T-010", Type: "chore", Role: "worker", Title: "overlap A", Owns: []string{"internal/store/**"}, Status: StatusReady, Budget: Budget{Steps: 10, WallMin: 5}},
		{ID: "T-011", Type: "chore", Role: "worker", Title: "overlap B", Owns: []string{"internal/store/**"}, Status: StatusReady, Budget: Budget{Steps: 10, WallMin: 5}},
	}
	g := Graph{Tasks: tasks}
	for i := range g.Tasks {
		pkt := fmt.Sprintf("# %s\n\n%s\n", tasks[i].ID, tasks[i].Title)
		p := filepath.Join(clone, "harness", "tasks", tasks[i].ID+".md")
		if err := os.WriteFile(p, []byte(pkt), 0o644); err != nil {
			return err
		}
	}
	if err := saveGraph(clone, g); err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Join(clone, "harness", "handoffs"), 0o755)

	// Manifest + ratchet baseline BEFORE the commit so worktrees (branch from
	// main) carry both, and guard's checksum check can see tampering.
	if err := GenerateManifest(clone); err != nil {
		return err
	}
	bl := baseline{Go: countGoTests(clone), Pw: countPwTests(clone)}
	b, _ := json.Marshal(bl)
	if err := os.WriteFile(filepath.Join(clone, "harness", ".ratchet.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}

	// Commit fixture.
	_ = gitOut2(clone, "add", "-A")
	if _, err := gitOut(clone, "commit", "-m", "test(harness): selftest fixtures"); err != nil {
		return err
	}
	return nil
}

// worktreeFor creates a fresh worktree+branch for a task from main.
func worktreeFor(clone, id string) (string, error) {
	wt := filepath.Join(filepath.Dir(clone), "wt", id)
	_ = os.MkdirAll(filepath.Dir(wt), 0o755)
	if out, err := gitOut(clone, "worktree", "add", wt, "-b", "task/"+id, "main"); err != nil {
		return "", fmt.Errorf("worktree add: %v: %s", err, out)
	}
	linkNodeModules(clone, wt)
	return wt, nil
}

func commitAll(wt, msg string) error {
	_ = gitOut2(wt, "add", "-A")
	_, err := gitOut(wt, "commit", "-m", msg)
	return err
}

func handoffFor(wt, id string) {
	p := filepath.Join(wt, "harness", "handoffs", id+".md")
	_ = os.WriteFile(p, []byte("done\n"), 0o644)
}

func scTamper(clone string, pol Policy) (bool, string) {
	g, _ := loadGraph(clone)
	t := g.task("T-000")
	wt, err := worktreeFor(clone, "T-000")
	if err != nil {
		return false, err.Error()
	}
	defer func() { _, _ = gitOut(clone, "worktree", "remove", "--force", wt) }()

	spec := filepath.Join(wt, "acceptance", "e2e", "version.spec.ts")
	f, _ := os.OpenFile(spec, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("// tampered\n")
	_ = f.Close()
	handoffFor(wt, "T-000")
	if err := commitAll(wt, "feat: tamper spec"); err != nil {
		return false, err.Error()
	}
	code := guardTask(clone, wt, t, pol)
	detail := fmt.Sprintf("guard exit=%d (want 3)", code)
	return code == exitProtected, detail
}

func scOwnership(clone string, pol Policy) (bool, string) {
	g, _ := loadGraph(clone)
	t := g.task("T-S3")
	wt, err := worktreeFor(clone, "T-S3")
	if err != nil {
		return false, err.Error()
	}
	defer func() { _, _ = gitOut(clone, "worktree", "remove", "--force", wt) }()

	if err := os.WriteFile(filepath.Join(wt, "README-extra.md"), []byte("stray\n"), 0o644); err != nil {
		return false, err.Error()
	}
	handoffFor(wt, "T-S3")
	if err := commitAll(wt, "chore: stray file"); err != nil {
		return false, err.Error()
	}
	code := guardTask(clone, wt, t, pol)
	detail := fmt.Sprintf("guard exit=%d (want 4)", code)
	return code == exitOwnership, detail
}

func scDeletedTest(clone string, pol Policy) (bool, string) {
	g, _ := loadGraph(clone)
	t := g.task("T-S4")
	wt, err := worktreeFor(clone, "T-S4")
	if err != nil {
		return false, err.Error()
	}
	defer func() { _, _ = gitOut(clone, "worktree", "remove", "--force", wt) }()

	if err := os.Remove(filepath.Join(wt, "internal", "web", "dummy_fixture_test.go")); err != nil {
		return false, err.Error()
	}
	handoffFor(wt, "T-S4")
	if err := commitAll(wt, "chore: delete test"); err != nil {
		return false, err.Error()
	}
	code := guardTask(clone, wt, t, pol)
	detail := fmt.Sprintf("guard exit=%d (want 5)", code)
	return code == exitRatchet, detail
}

func scRedCheck(clone string, pol Policy) (bool, string) {
	g, _ := loadGraph(clone)
	t := g.task("T-S5")
	mainBefore := strings.TrimSpace(gitOut2(clone, "rev-parse", "main"))
	wt, err := worktreeFor(clone, "T-S5")
	if err != nil {
		return false, err.Error()
	}
	defer func() { _, _ = gitOut(clone, "worktree", "remove", "--force", wt) }()

	red := `package web

import "testing"

func TestRedCheckScenario(t *testing.T) {
	t.Fatal("intentional red for the gate scenario")
}
`
	if err := os.WriteFile(filepath.Join(wt, "internal", "web", "red_scenario_test.go"), []byte(red), 0o644); err != nil {
		return false, err.Error()
	}
	handoffFor(wt, "T-S5")
	if err := commitAll(wt, "feat: failing test"); err != nil {
		return false, err.Error()
	}

	// Gate must refuse (make test-unit fails) and leave main unchanged.
	_ = t
	cmd := exec.Command(os.Args[0], "gate", "T-S5")
	cmd.Dir = clone
	cmd.Env = append(os.Environ(), "CHECK_TARGET=test-unit")
	out, _ := cmd.CombinedOutput()
	mainAfter := strings.TrimSpace(gitOut2(clone, "rev-parse", "main"))
	ok := mainBefore == mainAfter
	detail := fmt.Sprintf("gate refused, main unchanged=%v (out: %s)", ok, lastLines(string(out), 3))
	return ok, detail
}

func scOverlap(clone string, pol Policy) (bool, string) {
	// sched --dry-run in a copy of the clone with only T-010/T-011 left.
	tmp, _ := os.MkdirTemp("", "gl-dryrun-")
	defer os.RemoveAll(tmp)
	dr := filepath.Join(tmp, "repo")
	if out, err := gitOut("", "clone", clone, dr); err != nil {
		return false, out
	}
	g, _ := loadGraph(dr)
	for i := range g.Tasks {
		if g.Tasks[i].ID != "T-010" && g.Tasks[i].ID != "T-011" {
			g.Tasks[i].Status = StatusDone
		}
	}
	_ = saveGraph(dr, g)
	cmd := exec.Command(os.Args[0], "sched", "--dry-run")
	cmd.Dir = dr
	out, _ := cmd.CombinedOutput()
	s := string(out)
	together := strings.Contains(s, "wave 1: T-010 T-011")
	ok := !together && strings.Contains(s, "T-010") && strings.Contains(s, "T-011")
	detail := fmt.Sprintf("waves=%q overlap-avoided=%v", strings.TrimSpace(s), ok)
	return ok, detail
}

func scDirty(clone string, pol Policy) (bool, string) {
	g, _ := loadGraph(clone)
	t := g.task("T-S9")
	wt, err := worktreeFor(clone, "T-S9")
	if err != nil {
		return false, err.Error()
	}
	defer func() { _, _ = gitOut(clone, "worktree", "remove", "--force", wt) }()

	codeChange := `package web

// dirty_scenario is a legitimate committed change.
func dirtyScenario() string { return "ok" }
`
	if err := os.WriteFile(filepath.Join(wt, "internal", "web", "dirty_scenario.go"), []byte(codeChange), 0o644); err != nil {
		return false, err.Error()
	}
	handoffFor(wt, "T-S9")
	if err := commitAll(wt, "chore: committed change"); err != nil {
		return false, err.Error()
	}
	// Uncommitted tampering with the Makefile (E3).
	mk := filepath.Join(wt, "Makefile")
	b, _ := os.ReadFile(mk)
	_ = os.WriteFile(mk, append(b, []byte("\n# tampered\n")...), 0o644)

	code := guardTask(clone, wt, t, pol)
	detail := fmt.Sprintf("guard exit=%d (want 9)", code)
	return code == exitDirty, detail
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
