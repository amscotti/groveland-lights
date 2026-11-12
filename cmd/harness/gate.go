package main

import (
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

	// 1. Serial merge lock.
	lockPath := filepath.Join(root, "harness", ".merge.lock")
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("merge lock: %w", err)
	}

	// Precondition (E4): the main checkout must be clean.
	if dirty := DirtyLines(gitOut2(root, "status", "--porcelain")); len(dirty) > 0 {
		for _, d := range dirty {
			fmt.Fprintf(os.Stderr, "ERROR: main checkout dirty: %s\n", d)
		}
		return fmt.Errorf("main is dirty; an agent may have reached the main checkout")
	}

	// 2. Rebase onto main.
	if out, err := gitOut(wt, "rebase", "main"); err != nil {
		progress(root, "gate_conflict "+t.ID)
		// Try the integrator agent (live), limited to owns.
		if err := runIntegrator(root, wt, t, pol); err != nil {
			_, _ = gitOut(wt, "rebase", "--abort")
			setTaskStatus(root, t.ID, StatusBlocked, "rebase conflict unresolved")
			return fmt.Errorf("rebase conflict, integrator failed: %v (%s)", err, strings.TrimSpace(out))
		}
	}

	// 3. Guard re-run (includes clean-worktree check).
	if code := guardTask(root, wt, t, pol); code != 0 {
		progress(root, fmt.Sprintf("gate_guard_fail %s exit=%d", t.ID, code))
		return fmt.Errorf("guard failed with exit %d", code)
	}

	// 4. Full make check with the flake policy (E10).
	checkTarget := os.Getenv("CHECK_TARGET") // selftest only; never set in production
	if checkTarget == "" {
		checkTarget = "check"
	}
	if err := runMake(root, wt, checkTarget, pol); err != nil {
		progress(root, fmt.Sprintf("gate_check_fail %s", t.ID))
		return fmt.Errorf("make %s failed: %w", checkTarget, err)
	}

	// 5. Holdout suite (absolute path per E1).
	if pol.HoldoutDir != "" {
		if _, err := os.Stat(pol.HoldoutDir); err == nil {
			if err := runMake(root, wt, "holdout", pol); err != nil {
				progress(root, fmt.Sprintf("gate_holdout_fail %s", t.ID))
				return fmt.Errorf("make holdout failed: %w", err)
			}
		}
	}

	// 6. ff-only merge into main.
	if _, err := gitOut(root, "merge", "--ff-only", "task/"+t.ID); err != nil {
		return fmt.Errorf("ff-only merge refused (diverged?): %w", err)
	}

	// 7. Push. The script pushes; agents never do.
	if out, err := gitOut(root, "push", "origin", "main"); err != nil {
		return fmt.Errorf("push failed: %v: %s", err, strings.TrimSpace(out))
	}

	sha := strings.TrimSpace(gitOut2(root, "rev-parse", "HEAD"))
	progress(root, "gate_ok "+t.ID)
	progress(root, "merged "+t.ID+" "+sha)

	// 8. Remove the worktree and branch.
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
	cmd := exec.Command("make", target)
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
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
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
