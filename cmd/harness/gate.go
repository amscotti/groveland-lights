package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// cmdGate implements the serial merge gate (plan §6.1.6). main is never left
// half-merged: any failure exits before the ff-only merge.
func cmdGate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: harness gate T-###")
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

	lock, err := takeMergeLock(root)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	if err := requireCleanMain(root); err != nil {
		return err
	}
	if err := rebaseOntoMain(root, wt, t, pol); err != nil {
		return err
	}
	if code := guardTask(root, wt, t, pol); code != 0 {
		progress(root, fmt.Sprintf("gate_guard_fail %s exit=%d", t.ID, code))
		return fmt.Errorf("guard failed with exit %d", code)
	}
	if err := gateChecks(root, wt, pol); err != nil {
		progress(root, fmt.Sprintf("gate_check_fail %s", t.ID))
		return err
	}
	return finalizeMerge(root, wt, t)
}

func takeMergeLock(root string) (*os.File, error) {
	lf, err := os.OpenFile(filepath.Join(root, "harness", ".merge.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return nil, fmt.Errorf("merge lock: %w", err)
	}
	return lf, nil
}

// requireCleanMain enforces the E4 precondition. Scheduler-owned state files
// (graph, progress, reviews, baselines) are expected to be in flux during a
// run; the assertion exists to catch AGENT edits to main.
func requireCleanMain(root string) error {
	dirty := dirtyNonScheduler(root)
	if len(dirty) == 0 {
		return nil
	}
	for _, d := range dirty {
		fmt.Fprintf(os.Stderr, "ERROR: main checkout dirty: %s\n", d)
	}
	return fmt.Errorf("main is dirty; an agent may have reached the main checkout")
}

// rebaseOntoMain is gate step 2, with the integrator fallback.
func rebaseOntoMain(root, wt string, t *Task, pol Policy) error {
	if out, err := gitOut(wt, "rebase", "main"); err != nil {
		progress(root, "gate_conflict "+t.ID)
		if err := runIntegrator(root, wt, t, pol); err != nil {
			_, _ = gitOut(wt, "rebase", "--abort")
			setTaskStatus(root, t.ID, StatusBlocked, "rebase conflict unresolved")
			return fmt.Errorf("rebase conflict, integrator failed: %w (%s)", err, strings.TrimSpace(out))
		}
	}
	return nil
}

// gateChecks runs make check (with the flake policy) and the holdout suite.
func gateChecks(root, wt string, pol Policy) error {
	checkTarget := os.Getenv("CHECK_TARGET") // selftest only; never set in production
	if checkTarget == "" {
		checkTarget = "check"
	}
	if err := runMake(root, wt, checkTarget, pol); err != nil {
		return fmt.Errorf("make %s failed: %w", checkTarget, err)
	}
	if pol.HoldoutDir == "" {
		return nil
	}
	if _, err := os.Stat(pol.HoldoutDir); err != nil {
		return nil
	}
	if err := runMake(root, wt, "holdout", pol); err != nil {
		return fmt.Errorf("make holdout failed: %w", err)
	}
	return nil
}

// finalizeMerge: ff-only merge, push, progress log, worktree removal.
func finalizeMerge(root, wt string, t *Task) error {
	// graph.json is scheduler-owned: restore main's authoritative copy on the
	// branch so the merge cannot regress task status/attempts.
	if _, err := gitOut(wt, "checkout", "main", "--", "harness/tasks/graph.json"); err == nil {
		if out := gitOut2(wt, "status", "--porcelain"); strings.Contains(out, "graph.json") {
			_, _ = gitOut(wt, "add", "harness/tasks/graph.json")
			_, _ = gitOut(wt, "commit", "-m", "chore(harness): keep scheduler graph state on merge")
		}
	}
	if _, err := gitOut(root, "merge", "--ff-only", "task/"+t.ID); err != nil {
		return fmt.Errorf("ff-only merge refused (diverged?): %w", err)
	}
	if out, err := gitOut(root, "push", "origin", "main"); err != nil {
		return fmt.Errorf("push failed: %w: %s", err, strings.TrimSpace(out))
	}
	sha := strings.TrimSpace(gitOut2(root, "rev-parse", "HEAD"))
	progress(root, "gate_ok "+t.ID)
	progress(root, "merged "+t.ID+" "+sha)
	if out, err := gitOut(root, "worktree", "remove", "--force", wt); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: worktree remove: %v %s\n", err, out)
	}
	_, _ = gitOut(root, "branch", "-D", "task/"+t.ID)
	setTaskStatus(root, t.ID, StatusDone, "")
	return nil
}

// runMake runs a make target in the worktree, applying the flake policy: one retry
// of a failed Playwright target (e2e/a11y/visual), logged as a flake (E10).
func runMake(root, wt, target string, pol Policy) error {
	cmd := makeCmd(wt, target, pol)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s\n", truncate(string(out), 4000))
	if target == "check" {
		// Which Playwright sub-target failed? Retry each once.
		for _, pw := range []string{"e2e", "a11y", "visual"} {
			if strings.Contains(string(out), "make["+pw+"]") || strings.Contains(string(out), "make: *** ["+pw+"]") {
				fmt.Fprintf(os.Stderr, "gate: flake policy: retrying make %s once\n", pw)
				if err2 := runMakeSimple(wt, pw, pol); err2 == nil {
					progress(root, "flake "+target+"->"+pw)
					// Re-run the full target to confirm everything else passed.
					return runMakeSimple(wt, target, pol)
				}
			}
		}
	}
	return err
}

func runMakeSimple(wt, target string, pol Policy) error {
	cmd := makeCmd(wt, target, pol)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", truncate(string(out), 2000))
	}
	return err
}

func makeCmd(wt, target string, pol Policy) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), "make", target)
	cmd.Dir = wt
	cmd.Env = append(os.Environ(),
		"HOLDOUT_DIR="+pol.HoldoutDir,
		"CI=1",
	)
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func progress(root, event string) {
	f := filepath.Join(root, "harness", "progress.log")
	line := time.Now().UTC().Format(time.RFC3339) + " " + event + "\n"
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = fh.Close() }()
	_, _ = fh.WriteString(line)
}

func setTaskStatus(root, id, status, note string) {
	g, err := loadGraph(root)
	if err != nil {
		return
	}
	t := g.task(id)
	if t == nil {
		return
	}
	t.Status = status
	if note != "" {
		t.Findings = note
	}
	_ = saveGraph(root, g)
}
