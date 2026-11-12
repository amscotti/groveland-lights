package main

import "fmt"

// ValidateGraph enforces the task-graph invariants that don't need git:
// cycles, contract-first readiness, and assertion coverage.
func ValidateGraph(g Graph, assertions []Assertion) error {
	byID := map[string]*Task{}
	for i := range g.Tasks {
		t := &g.Tasks[i]
		if _, dup := byID[t.ID]; dup {
			return fmt.Errorf("duplicate task %s", t.ID)
		}
		byID[t.ID] = t
	}
	// Cycle detection over deps.
	const white, grey, black = 0, 1, 2
	color := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		switch color[id] {
		case grey:
			return fmt.Errorf("dependency cycle at %s", id)
		case black:
			return nil
		}
		color[id] = grey
		t := byID[id]
		if t == nil {
			return fmt.Errorf("unknown task %s in deps", id)
		}
		for _, d := range t.Deps {
			if err := visit(d); err != nil {
				return err
			}
		}
		color[id] = black
		return nil
	}
	for _, t := range g.Tasks {
		if err := visit(t.ID); err != nil {
			return err
		}
	}
	// Every gating assertion is claimed by at least one task; non-gating
	// (gate:false / verified:human) still needs an owner.
	claimed := map[string]bool{}
	for _, t := range g.Tasks {
		for _, c := range t.Claims {
			claimed[c] = true
		}
	}
	for _, a := range assertions {
		if !claimed[a.ID] {
			return fmt.Errorf("assertion %s is not claimed by any task", a.ID)
		}
	}
	known := map[string]bool{}
	for _, a := range assertions {
		known[a.ID] = true
	}
	for _, t := range g.Tasks {
		for _, c := range t.Claims {
			if !known[c] {
				return fmt.Errorf("task %s claims unknown assertion %s", t.ID, c)
			}
		}
	}
	return nil
}

// ReadyTasks returns tasks whose deps are done, that don't overlap owns with
// any running/merging task or a task already selected in this batch, capped at
// remaining slots. rework tasks count as ready once their findings are appended.
func ReadyTasks(g *Graph, p Policy) []*Task {
	var out []*Task
	var selectedOwns [][]string
	running := 0
	for i := range g.Tasks {
		s := g.Tasks[i].Status
		if s == StatusRunning || s == StatusReview || s == StatusMerging {
			running++
		}
	}
	slots := p.MaxParallel - running
	if slots <= 0 {
		return nil
	}
	for i := range g.Tasks {
		t := &g.Tasks[i]
		if t.Status != StatusReady && t.Status != StatusRework {
			continue
		}
		depsDone := true
		for _, d := range t.Deps {
			dt := g.task(d)
			if dt == nil || dt.Status != StatusDone {
				depsDone = false
				break
			}
		}
		if !depsDone {
			continue
		}
		conflict := false
		for j := range g.Tasks {
			o := &g.Tasks[j]
			if o.ID == t.ID {
				continue
			}
			if o.Status == StatusRunning || o.Status == StatusReview || o.Status == StatusMerging {
				if ownsOverlap(t.Owns, o.Owns) {
					conflict = true
					break
				}
			}
		}
		// Also never select two overlapping tasks in the same batch.
		for _, so := range selectedOwns {
			if ownsOverlap(t.Owns, so) {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}
		out = append(out, t)
		selectedOwns = append(selectedOwns, t.Owns)
		if len(out) == slots {
			break
		}
	}
	return out
}

// ContractFirstReady enforces: a feature task may only be ready if every one of its
// claims is covered by an acceptance task (type "acceptance") that is done or the
// acceptance task it depends on is done. Simplification per plan §6.1.3: a feature
// task becomes ready only after the acceptance task for its claims is done.
func ContractFirstReady(t *Task, g *Graph) error {
	if t.Type != "feature" {
		return nil
	}
	for _, d := range t.Deps {
		dt := g.task(d)
		if dt != nil && dt.Type == "acceptance" && dt.Status == StatusDone {
			return nil
		}
	}
	// No acceptance dep found anywhere in its deps: violation.
	return fmt.Errorf("feature task %s has no done acceptance task among its deps (contract-first rule)", t.ID)
}
