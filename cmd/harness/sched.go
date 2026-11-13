package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// cmdSched is the scheduler loop (plan §6.1.4). Deterministic: no LLM decides
// anything here; agents are spawned as subprocesses and their output is judged
// mechanically (guard, verdict files, gate).
func cmdSched(args []string) error {
	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		}
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
	assertions, err := loadAssertions(root)
	if err != nil {
		return fmt.Errorf("contract.json: %w", err)
	}
	if err := ValidateGraph(g, assertions); err != nil {
		return fmt.Errorf("graph invalid: %w", err)
	}

	if dryRun {
		return schedDryRun(root, pol)
	}
	if err := schedPreconditions(root, pol); err != nil {
		if errors.Is(err, errStopRequested) {
			return nil
		}
		return err
	}

	var wg sync.WaitGroup
	return schedLoop(root, pol, &wg)
}

func schedLoop(root string, pol Policy, wg *sync.WaitGroup) error {
	for {
		g, err := loadGraph(root)
		if err != nil {
			return err
		}
		if stopExists(root) {
			fmt.Println("STOP signaled; draining")
			wg.Wait()
			return nil
		}
		if err := dispatchReady(root, pol, &g, wg); err != nil {
			return err
		}
		if countActive(&g) > 0 {
			time.Sleep(10 * time.Second)
			continue
		}
		if len(ReadyTasks(&g, pol)) == 0 {
			fmt.Println("no ready or active tasks remain")
			return nil
		}
	}
}

func schedPreconditions(root string, pol Policy) error {
	if _, err := os.Stat(filepath.Join(root, "harness", "STOP")); err == nil {
		fmt.Println("harness/STOP exists; not starting")
		return errStopRequested
	}
	// start-up: holdout path is absolute by construction (policy.json).
	if pol.HoldoutDir != "" && !filepath.IsAbs(pol.HoldoutDir) {
		return fmt.Errorf("policy holdout_dir must be absolute (E1), got %q", pol.HoldoutDir)
	}
	return nil
}

// errStopRequested signals a clean exit when harness/STOP exists.
var errStopRequested = errors.New("stop requested")

// dispatchReady claims the currently-ready tasks. Returns an error only for
// stop-the-world conditions (dirty main, bad state).
func dispatchReady(root string, pol Policy, g *Graph, wg *sync.WaitGroup) error {
	ready := ReadyTasks(g, pol)
	for _, t := range ready {
		// Contract-first (feature tasks need a done acceptance dep).
		if err := ContractFirstReady(t, g); err != nil {
			setTaskStatus(root, t.ID, StatusBlocked, err.Error())
			progress(root, "blocked "+t.ID+" "+err.Error())
			continue
		}
		// E4: main must be clean before claiming.
		if dirty := DirtyLines(gitOut2(root, "status", "--porcelain")); len(dirty) > 0 {
			return fmt.Errorf("main checkout dirty: %v — stopping the world", dirty)
		}
		// E9: commit graph + new packets before creating worktrees.
		if err := commitHarnessState(root); err != nil {
			return err
		}
		if err := spawnTask(root, pol, t, wg); err != nil {
			setTaskStatus(root, t.ID, StatusBlocked, err.Error())
			progress(root, "blocked "+t.ID+" "+err.Error())
		}
	}
	return nil
}

func stopExists(root string) bool {
	_, err := os.Stat(filepath.Join(root, "harness", "STOP"))
	return err == nil
}

func commitHarnessState(root string) error {
	if out := gitOut2(root, "status", "--porcelain", "harness/tasks", "harness/progress.log"); strings.TrimSpace(out) == "" {
		return nil
	}
	_ = gitOut2(root, "add", "harness/tasks", "harness/progress.log")
	if _, err := gitOut(root, "commit", "-m", "chore(harness): update task graph and progress"); err != nil {
		// Nothing to commit is fine; anything else is not.
		if !strings.Contains(err.Error(), "nothing to commit") {
			return err
		}
	}
	return nil
}

// spawnTask runs one task through worker -> guard -> check-fast -> reviews -> gate.
// A rework reuses the existing worktree (the agent's uncommitted work survives).
func spawnTask(root string, pol Policy, t *Task, wg *sync.WaitGroup) error {
	wt := filepath.Join(filepath.Dir(root), "wt", t.ID)
	if _, err := os.Stat(wt); err == nil {
		if out, err2 := gitOut(wt, "rev-parse", "--abbrev-ref", "HEAD"); err2 == nil && strings.TrimSpace(out) == "task/"+t.ID {
			// Reuse for rework: reset the branch tip to main only if there are
			// no commits (nothing to lose); otherwise keep the branch as-is.
			progress(root, "reuse_worktree "+t.ID)
		} else {
			return fmt.Errorf("worktree %s exists but not on task branch", wt)
		}
	} else if out, err := gitOut(root, "worktree", "add", wt, "-b", "task/"+t.ID, "main"); err != nil {
		return fmt.Errorf("worktree add: %w: %s", err, out)
	}
	linkNodeModules(root, wt)
	if _, err := os.Stat(filepath.Join(root, ".env.test")); err == nil {
		_ = copyFile(filepath.Join(root, ".env.test"), filepath.Join(wt, ".env.test"))
	}

	setTaskStatus(root, t.ID, StatusRunning, "")
	progress(root, "claimed "+t.ID+" "+t.Role)

	wg.Add(1)
	go func() {
		defer wg.Done()
		runTaskLifecycle(root, pol, t.ID)
	}()
	return nil
}

func linkNodeModules(root, wt string) {
	src := filepath.Join(root, "node_modules")
	dst := filepath.Join(wt, "node_modules")
	if _, err := os.Stat(src); err == nil {
		_ = os.Symlink(src, dst)
	}
}

// runTaskLifecycle owns one task from worker spawn to gate (or rework/blocked).
func runTaskLifecycle(root string, pol Policy, id string) {
	g, err := loadGraph(root)
	if err != nil {
		return
	}
	t := g.task(id)
	if t == nil {
		return
	}

	// Spawn the worker agent (or the escalation model on retries).
	modelLogical := t.modelLogical(pol)
	role := t.Role
	transcript := filepath.Join(root, "harness", "runs", timestamp(), id, role+".jsonl")
	_ = os.MkdirAll(filepath.Dir(transcript), 0o750)

	msg := fmt.Sprintf("Execute the attached task packet. Follow AGENTS.md. Write harness/handoffs/%s.md.", id)
	if err := runAgent(root, wt(root, id), role, modelLogical, id, filepath.Join(root, "harness", "tasks", id+".md"), msg, transcript, t.Budget.WallMin); err != nil {
		progress(root, fmt.Sprintf("worker_error %s %v", id, err))
		reworkOrBlock(root, pol, t, fmt.Sprintf("agent run failed: %v", err))
		return
	}
	progress(root, "worker_done "+id)

	// E13: assert which model actually served the run.
	checkServedModel(root, pol, role, modelLogical, transcript)

	// Guard.
	gg, _ := loadGraph(root)
	tk := gg.task(id)
	if code := guardTask(root, wt(root, id), tk, pol); code != 0 {
		progress(root, fmt.Sprintf("guard_fail %s exit=%d", id, code))
		reworkOrBlock(root, pol, tk, fmt.Sprintf("guard exit %d", code))
		return
	}
	progress(root, "guard_ok "+id)

	// check-fast in the worktree.
	if err := runMakeSimple(wt(root, id), "check-fast", pol); err != nil {
		progress(root, "check_fast_fail "+id)
		reworkOrBlock(root, pol, tk, "make check-fast failed")
		return
	}
	progress(root, "check_fast_ok "+id)

	// Reviews (parallel): reviewer + reviewer-alt (+ visual-reviewer if ui).
	diffFile := filepath.Join(root, "harness", "reviews", id+".diff")
	_ = os.MkdirAll(filepath.Dir(diffFile), 0o750)
	_ = os.WriteFile(diffFile, []byte(gitOut2(wt(root, id), "diff", "main...HEAD")), 0o600)

	setTaskStatus(root, id, StatusReview, "")
	roles := []string{"reviewer", "reviewer-alt"}
	if tk.UI {
		roles = append(roles, "visual-reviewer")
	}
	var rwg sync.WaitGroup
	verdicts := make(map[string]string)
	var mu sync.Mutex
	for _, rr := range roles {
		rwg.Add(1)
		go func(rr string) {
			defer rwg.Done()
			v := runReview(root, wt(root, id), id, rr, pol)
			mu.Lock()
			verdicts[rr] = v
			mu.Unlock()
		}(rr)
	}
	rwg.Wait()

	blocked := false
	for _, rr := range roles {
		v := verdicts[rr]
		if v == "PASS" {
			progress(root, "review_pass("+rr+") "+id)
			continue
		}
		blocked = true
		progress(root, fmt.Sprintf("review_block(%s) %s", rr, id))
	}

	if blocked {
		appendFindings(root, id, roles, verdicts)
		reworkOrBlock(root, pol, tk, "review BLOCK")
		return
	}

	// Gate.
	setTaskStatus(root, id, StatusMerging, "")
	if err := cmdGate([]string{id}); err != nil {
		progress(root, "gate_fail "+id)
		reworkOrBlock(root, pol, tk, fmt.Sprintf("gate: %v", err))
		return
	}
}

// reworkOrBlock: attempts++ -> rework, or escalate once, or blocked after max attempts.
func reworkOrBlock(root string, pol Policy, t *Task, reason string) {
	g, _ := loadGraph(root)
	tk := g.task(t.ID)
	if tk == nil {
		return
	}
	tk.Attempts++
	if tk.Attempts >= pol.MaxAttempts {
		// Escalate model once (plan §6.1.1 rule 2), then block.
		esc := pol.EscalateWorker
		if tk.Role == "ui-worker" {
			esc = pol.EscalateUIWorker
		}
		if tk.Model == "" && esc != "" {
			tk.Model = esc
			tk.Status = StatusRework
			progress(root, "escalate "+tk.ID+" "+esc)
		} else {
			tk.Status = StatusBlocked
			tk.Findings = reason
			progress(root, "blocked "+tk.ID+" "+reason)
		}
	} else {
		tk.Status = StatusRework
		appendRetryContext(root, tk.ID, reason)
		progress(root, "rework "+tk.ID+" attempt="+fmt.Sprint(tk.Attempts)+" "+reason)
	}
	_ = saveGraph(root, g)
}

// appendRetryContext tells the next attempt WHY the previous one failed.
func appendRetryContext(root, id, reason string) {
	pkt := filepath.Join(root, "harness", "tasks", id+".md")
	fh, err := os.OpenFile(pkt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = fh.Close() }()
	fmt.Fprintf(fh, "\n\n## Retry context (attempt failed — read this)\n\nThe previous attempt failed for this reason: %s\nFix it and finish. Your earlier work may already be in the worktree uncommitted — verify, complete the required steps (including `git add` + `git commit -m \"...\"`), and write the handoff.\n", reason)
}

func appendFindings(root, id string, roles []string, verdicts map[string]string) {
	var b strings.Builder
	b.WriteString("\n\n## Review findings\n")
	for _, rr := range roles {
		if verdicts[rr] != "PASS" {
			f := filepath.Join(root, "harness", "reviews", id+"-"+rr+".md")
			if data, err := os.ReadFile(f); err == nil {
				b.WriteString("\n### " + rr + "\n" + string(data) + "\n")
			} else {
				b.WriteString("\n### " + rr + "\n(no review file; verdict " + verdicts[rr] + ")\n")
			}
		}
	}
	pkt := filepath.Join(root, "harness", "tasks", id+".md")
	fh, err := os.OpenFile(pkt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = fh.Close() }()
	_, _ = fh.WriteString(b.String())
}

// runReview spawns a review agent and parses its verdict.
func runReview(root, wt, id, role string, pol Policy) string {
	out := filepath.Join(root, "harness", "reviews", id+"-"+role+".md")
	transcript := filepath.Join(root, "harness", "runs", timestamp(), id, role+".jsonl")
	_ = os.MkdirAll(filepath.Dir(transcript), 0o750)

	diffPath := filepath.Join(root, "harness", "reviews", id+".diff")
	extraFiles := []string{diffPath}
	if role == "visual-reviewer" {
		// Attach the key screenshots if any exist under test-results.
		extraFiles = screenshots(wt)
	}
	msg := fmt.Sprintf("Review task %s. Diff or screenshots attached. Write your findings and verdict to %s.",
		id, "harness/reviews/"+id+"-"+role+".md")
	if err := runAgent(root, root, role, pol.Roles[role], id, extraFiles[0], msg, transcript, 30); err != nil {
		return "ERROR: " + err.Error()
	}
	// Verdict: prefer the review file, fall back to the transcript.
	if data, err := os.ReadFile(out); err == nil {
		if v, ok := ParseVerdict(string(data)); ok {
			return v
		}
	}
	if data, err := os.ReadFile(transcript); err == nil {
		if v, ok := ParseVerdict(string(data)); ok {
			return v
		}
	}
	return "BLOCK" // no verdict = block
}

func screenshots(wt string) []string {
	var out []string
	_ = filepath.Walk(filepath.Join(wt, "test-results"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".png") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// runAgent spawns `opencode run` and streams the JSON transcript to a file.
func runAgent(mainRoot, dir, role, modelLogical, title, file, msg, transcriptPath string, wallMin int) error {
	modelID, ok := modelIDFor(mainRoot, modelLogical)
	if !ok {
		return fmt.Errorf("no model id for %q", modelLogical)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(wallMin+5)*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "run",
		"--standalone", "--auto",
		"--agent", role,
		"--model", modelID,
		"--format", "json",
		"--archived",
		"--title", title,
		"--file", file,
		msg,
	)
	cmd.Dir = dir
	outFh, err := os.Create(transcriptPath)
	if err != nil {
		return err
	}
	defer func() { _ = outFh.Close() }()
	cmd.Stdout = outFh
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func modelIDFor(root, logical string) (string, bool) {
	pol, err := loadPolicy(root)
	if err != nil {
		return "", false
	}
	id, ok := pol.Models[logical]
	return id, ok
}

// checkServedModel asserts the model that served a run matches policy (E13).
func checkServedModel(root string, pol Policy, role, wantLogical, transcript string) {
	data, err := os.ReadFile(transcript)
	if err != nil || len(data) == 0 {
		progress(root, "model_check WARN "+role+" no transcript")
		return
	}
	served := ServedModel(string(data))
	if len(served) == 0 {
		progress(root, "model_check WARN "+role+" no model field found in transcript")
		return
	}
	want := pol.Models[wantLogical]
	for _, s := range served {
		if s == want || strings.HasPrefix(s, want+"#") || strings.HasPrefix(want, s) {
			progress(root, "model_check ok "+role+" "+s)
			return
		}
	}
	progress(root, "model_check MISMATCH "+role+" want="+want+" got="+strings.Join(served, ","))
	fmt.Fprintf(os.Stderr, "ERROR: served model %v does not match policy %s for role %s\n", served, want, role)
}

func wt(root, id string) string {
	return filepath.Join(filepath.Dir(root), "wt", id)
}

var tsMu sync.Mutex
var tsLast string

func timestamp() string {
	tsMu.Lock()
	defer tsMu.Unlock()
	ts := time.Now().UTC().Format("20060102T150405")
	if ts == tsLast { // same second: disambiguate
		ts += "b"
	}
	tsLast = ts
	return ts
}

// schedDryRun simulates scheduling wave by wave and prints each wave; used by
// selftest S7 to prove overlapping-owns tasks never share a wave.
func schedDryRun(root string, pol Policy) error {
	g, err := loadGraph(root)
	if err != nil {
		return err
	}
	wave := 0
	for {
		ready := ReadyTasks(&g, pol)
		if len(ready) == 0 {
			if countActive(&g) == 0 {
				break
			}
			for i := range g.Tasks {
				if g.Tasks[i].Status == StatusRunning {
					g.Tasks[i].Status = StatusDone
				}
			}
			continue
		}
		wave++
		var ids []string
		for _, t := range ready {
			t.Status = StatusRunning
			ids = append(ids, t.ID)
		}
		fmt.Printf("wave %d: %s\n", wave, strings.Join(ids, " "))
		if wave > 200 {
			return fmt.Errorf("dry-run did not terminate")
		}
	}
	return nil
}

// helpers used by sched/gate shared code

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}

var reIntegratorPrompt = regexp.MustCompile(`integrator`)

func runIntegrator(root, wtPath string, t *Task, pol Policy) error {
	transcript := filepath.Join(root, "harness", "runs", timestamp(), t.ID, "integrator.jsonl")
	_ = os.MkdirAll(filepath.Dir(transcript), 0o750)
	conflicts := gitOut2(wtPath, "diff", "--name-only", "--diff-filter=U")
	msg := "Resolve the rebase conflicts in these files only: " + conflicts +
		" Task owns: " + strings.Join(t.Owns, ", ") + ". Preserve both sides' intent, run make check-fast, write harness/handoffs/" + t.ID + ".md."
	if err := runAgent(root, wtPath, "integrator", pol.Roles["integrator"], t.ID, filepath.Join(root, "harness", "tasks", t.ID+".md"), msg, transcript, 30); err != nil {
		return err
	}
	if reIntegratorPrompt.MatchString(conflicts) && conflicts == "" {
		return fmt.Errorf("no conflict files listed")
	}
	_, err := gitOut(wtPath, "rebase", "--continue")
	return err
}

var _ = bufio.NewReader // keep import if unused in some builds
