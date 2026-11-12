package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"internal/img/**", "internal/img/foo.go", true},
		{"internal/img/**", "internal/img/sub/bar.go", true},
		{"internal/img/**", "internal/geo/x.go", false},
		{"web/**", "web/templates/base.html", true},
		{"acceptance/**", "acceptance/go/healthz_test.go", true},
		{"cmd/server/**", "cmd/server/main.go", true},
		{"**/*.golden*", "testdata/schema.golden.sql", true}, // **/ also matches zero dirs
		{"**/*.golden*", "testdata/x/schema.golden.sql", true},
		{"Makefile", "Makefile", true},
		{"Makefile", "docs/Makefile", false},
		{"migrations/**", "migrations/0001_init.sql", true},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestOwnsOverlap(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{[]string{"internal/store/**"}, []string{"internal/store/**"}, true},
		{[]string{"internal/store/**"}, []string{"internal/img/**"}, false},
		{[]string{"internal/web/auth*.go"}, []string{"internal/web/**"}, true},
		{[]string{"web/templates/admin/**"}, []string{"web/templates/admin/moderation.html"}, true},
		{[]string{"cmd/server/**"}, []string{"internal/**"}, false},
	}
	for _, c := range cases {
		if got := ownsOverlap(c.a, c.b); got != c.want {
			t.Errorf("ownsOverlap(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func mkGraph(tasks ...Task) Graph {
	return Graph{Tasks: tasks}
}

func TestReadyTasksDepsAndSlots(t *testing.T) {
	pol := Policy{MaxParallel: 2}
	g := mkGraph(
		Task{ID: "T-1", Status: StatusDone, Owns: []string{"a/**"}},
		Task{ID: "T-2", Status: StatusReady, Deps: []string{"T-1"}, Owns: []string{"b/**"}},
		Task{ID: "T-3", Status: StatusReady, Deps: []string{"T-9"}, Owns: []string{"c/**"}},
		Task{ID: "T-4", Status: StatusReady, Owns: []string{"d/**"}},
	)
	ready := ReadyTasks(&g, pol)
	ids := map[string]bool{}
	for _, r := range ready {
		ids[r.ID] = true
	}
	if !ids["T-2"] || !ids["T-4"] {
		t.Errorf("ready = %v, want T-2 and T-4", ids)
	}
	if ids["T-3"] {
		t.Error("T-3 dep not done but scheduled")
	}
	// Slots: with one running, only one more fits.
	g2 := mkGraph(
		Task{ID: "T-R", Status: StatusRunning, Owns: []string{"z/**"}},
		Task{ID: "T-5", Status: StatusReady, Owns: []string{"e/**"}},
		Task{ID: "T-6", Status: StatusReady, Owns: []string{"f/**"}},
	)
	ready2 := ReadyTasks(&g2, pol)
	if len(ready2) != 1 {
		t.Errorf("slots: ready = %d, want 1", len(ready2))
	}
}

func TestOwnsOverlapBlocksParallel(t *testing.T) {
	pol := Policy{MaxParallel: 2}
	g := mkGraph(
		Task{ID: "T-A", Status: StatusRunning, Owns: []string{"internal/store/**"}},
		Task{ID: "T-B", Status: StatusReady, Owns: []string{"internal/store/**"}},
		Task{ID: "T-C", Status: StatusReady, Owns: []string{"internal/img/**"}},
	)
	ready := ReadyTasks(&g, pol)
	for _, r := range ready {
		if r.ID == "T-B" {
			t.Error("overlapping owns task was scheduled alongside running task")
		}
	}
}

func TestValidateGraphCycle(t *testing.T) {
	g := mkGraph(
		Task{ID: "T-1", Deps: []string{"T-2"}, Status: StatusReady},
		Task{ID: "T-2", Deps: []string{"T-1"}, Status: StatusReady},
	)
	if err := ValidateGraph(g, nil); err == nil {
		t.Error("cycle not detected")
	}
}

func TestValidateGraphAssertionCoverage(t *testing.T) {
	gate := true
	assertions := []Assertion{
		{ID: "A-1", Gate: &gate},
		{ID: "A-2", Gate: &gate},
	}
	g := mkGraph(Task{ID: "T-1", Claims: []string{"A-1"}})
	if err := ValidateGraph(g, assertions); err == nil {
		t.Error("unclaimed assertion not detected")
	}
	g2 := mkGraph(
		Task{ID: "T-1", Claims: []string{"A-1"}},
		Task{ID: "T-2", Claims: []string{"A-2"}},
	)
	if err := ValidateGraph(g2, assertions); err != nil {
		t.Errorf("valid graph rejected: %v", err)
	}
	// Unknown claim.
	g3 := mkGraph(Task{ID: "T-1", Claims: []string{"A-1", "A-999"}})
	if err := ValidateGraph(g3, assertions); err == nil {
		t.Error("unknown assertion claim not detected")
	}
}

func TestContractFirstReady(t *testing.T) {
	g := mkGraph(
		Task{ID: "T-ACC", Type: "acceptance", Status: StatusDone},
		Task{ID: "T-F1", Type: "feature", Deps: []string{"T-ACC"}},
		Task{ID: "T-F2", Type: "feature", Deps: []string{}},
	)
	if err := ContractFirstReady(g.task("T-F1"), &g); err != nil {
		t.Errorf("feature with done acceptance dep rejected: %v", err)
	}
	if err := ContractFirstReady(g.task("T-F2"), &g); err == nil {
		t.Error("feature without acceptance dep accepted")
	}
}

func TestRatchetCompare(t *testing.T) {
	if err := RatchetCompare(10, 11, 5, 5); err != nil {
		t.Errorf("increasing counts rejected: %v", err)
	}
	if err := RatchetCompare(10, 9, 5, 5); err == nil {
		t.Error("go count decrease accepted")
	}
	if err := RatchetCompare(10, 10, 5, 4); err == nil {
		t.Error("playwright count decrease accepted")
	}
}

func TestParseVerdict(t *testing.T) {
	cases := map[string]struct {
		v     string
		found bool
	}{
		"findings\nVERDICT: PASS":                    {"PASS", true},
		"VERDICT: BLOCK":                             {"BLOCK", true},
		"noise\nVERDICT: PASS\nmore":                 {"PASS", true},
		"first\nVERDICT: PASS\nthen\nVERDICT: BLOCK": {"BLOCK", true}, // last wins
		"no verdict here":                            {"", false},
		"embedded VERDICT: PASS mid-line":            {"", false}, // strict: line must start with VERDICT:
	}
	for in, want := range cases {
		v, ok := ParseVerdict(in)
		if ok != want.found || v != want.v {
			t.Errorf("ParseVerdict(%q) = (%q,%v), want (%q,%v)", in, v, ok, want.v, want.found)
		}
	}
}

func TestDirtyLines(t *testing.T) {
	porcelain := " M harness/handoffs/T-1.md\n?? harness/runs/x/y.jsonl\n M Makefile\n M node_modules\n?? .env.test\n"
	dirty := DirtyLines(porcelain)
	if len(dirty) != 1 || dirty[0] != " M Makefile" {
		t.Errorf("DirtyLines = %v, want [ M Makefile]", dirty)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "acceptance", "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acceptance", "go", "x_test.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateManifest(dir); err != nil {
		t.Fatal(err)
	}
	if err := VerifyManifest(dir); err != nil {
		t.Fatalf("clean tree failed verification: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acceptance", "go", "x_test.go"), []byte("package x // tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyManifest(dir); err == nil {
		t.Fatal("tampered file passed verification")
	}
}

func TestServedModel(t *testing.T) {
	tr := `{"type":"message","model":"zai-coding-plan/glm-5.3-flash"}
{"type":"part","model":"zai-coding-plan/glm-5.3-flash#std"}`
	got := ServedModel(tr)
	if len(got) == 0 || !contains(got, "zai-coding-plan/glm-5.3-flash") {
		t.Errorf("ServedModel = %v", got)
	}
}

func TestScanAddedLines(t *testing.T) {
	added := []addedLine{
		{path: "internal/web/x.go", n: 3, line: "// TODO later"},
		{path: "internal/web/x_test.go", n: 4, line: "// TODO in test ok"},
		{path: "web/static/js/app.js", n: 2, line: "console.log('x')"},
		{path: "internal/web/y.go", n: 9, line: "//nolint:gosec"},
	}
	findings := ScanAddedLines(added)
	if len(findings) != 3 {
		t.Fatalf("findings = %v, want 3", findings)
	}
}

func TestParseUnifiedAdded(t *testing.T) {
	diff := "+++ b/internal/web/x.go\n@@ -1,3 +1,4 @@\n ctx\n+new line one\n-old\n+new line two\n"
	added := ParseUnifiedAdded(diff)
	if len(added) != 2 {
		t.Fatalf("added = %+v, want 2", added)
	}
	if added[0].path != "internal/web/x.go" || added[0].line != "new line one" {
		t.Errorf("added[0] = %+v", added[0])
	}
	if added[0].n != 2 {
		t.Errorf("added[0].n = %d, want 2 (ctx is line 1)", added[0].n)
	}
	if added[1].n != 3 {
		t.Errorf("added[1].n = %d, want 3 (-old does not advance the new-file counter)", added[1].n)
	}
}

func TestDiffDepsGo(t *testing.T) {
	old := "module x\n\ngo 1.27\n"
	new := "module x\n\nrequire (\n\tmodernc.org/sqlite v1.62.0\n)\n"
	added := DiffDeps(old, new, "go")
	if len(added) == 0 {
		t.Fatal("no deps detected")
	}
	if !DepAllowed(added[0], []string{"modernc.org/sqlite"}) {
		t.Errorf("allowed dep flagged: %v", added)
	}
	if DepAllowed(added[0], []string{"github.com/other/mod"}) {
		t.Error("unallowed dep passed")
	}
}

func TestLastMatchDenyAll(t *testing.T) {
	rules := []permissionRule{
		{Action: "*", Resource: "*", Effect: "allow"}, // built-in base
		{Action: "*", Resource: "*", Effect: "deny"},  // agent deny-all
		{Action: "read", Resource: "*", Effect: "allow"},
		{Action: "subagent", Resource: "*", Effect: "deny"},
	}
	if eff := lastMatch(rules, "*", "*"); eff != "deny" {
		t.Errorf("lastMatch(*,*) = %q, want deny", eff)
	}
	if eff := lastMatch(rules, "subagent", "*"); eff != "deny" {
		t.Errorf("lastMatch(subagent,*) = %q, want deny", eff)
	}
	if eff := lastMatch(rules, "read", "*"); eff != "allow" {
		t.Errorf("lastMatch(read,*) = %q, want allow", eff)
	}
}

func TestSkelOverlap(t *testing.T) {
	if !globsOverlap("internal/web/auth*.go", "internal/web/**") {
		t.Error("prefix overlap missed")
	}
	if globsOverlap("internal/img/**", "internal/geo/**") {
		t.Error("disjoint globs flagged")
	}
}
