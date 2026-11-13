package main

import "fmt"

// ValidateGraph enforces the task-graph invariants that don't need git:
// cycles, contract-first readiness, and assertion coverage.
func ValidateGraph(g Graph, assertions []Assertion) error {
	byID := indexTasks(g)
	if err := checkNoCycles(g, byID); err != nil {
		return err
	}
	return checkClaimCoverage(g, assertions)
}

func indexTasks(g Graph) map[string]*Task {
	byID := map[string]*Task{}
	for i := range g.Tasks {
		byID[g.Tasks[i].ID] = &g.Tasks[i]
	}
	return byID
}

func checkNoCycles(g Graph, byID map[string]*Task) error {
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
	return nil
}

// checkClaimCoverage: every assertion is claimed by at least one task; no task
// claims an unknown assertion. Non-gating assertions still need an owner.
func checkClaimCoverage(g Graph, assertions []Assertion) error {
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
// remaining slots.
func ReadyTasks(g *Graph, p Policy) []*Task {
	var out []*Task
	var selectedOwns [][]string
	slots := p.MaxParallel - countActive(g)
	if slots <= 0 {
		return nil
	}
	for i := range g.Tasks {
		t := &g.Tasks[i]
		if !eligible(t, g) || conflictsActive(t, g) || conflictsBatch(t.Owns, selectedOwns) {
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

func countActive(g *Graph) int {
	n := 0
	for i := range g.Tasks {
		switch g.Tasks[i].Status {
		case StatusRunning, StatusReview, StatusMerging:
			n++
		}
	}
	return n
}

func eligible(t *Task, g *Graph) bool {
	if t.Status != StatusReady && t.Status != StatusRework {
		return false
	}
	for _, d := range t.Deps {
		dt := g.task(d)
		if dt == nil || dt.Status != StatusDone {
			return false
		}
	}
	return true
}

func conflictsActive(t *Task, g *Graph) bool {
	for j := range g.Tasks {
		o := &g.Tasks[j]
		if o.ID == t.ID {
			continue
		}
		if o.Status == StatusRunning || o.Status == StatusReview || o.Status == StatusMerging {
			if ownsOverlap(t.Owns, o.Owns) {
				return true
			}
		}
	}
	return false
}

func conflictsBatch(owns []string, selected [][]string) bool {
	for _, so := range selected {
		if ownsOverlap(owns, so) {
			return true
		}
	}
	return false
}

// ContractFirstReady enforces: a feature task may only be ready if an
// acceptance task is done among its deps — directly or transitively (feature
// tasks commonly depend only on sibling features that themselves depend on the
// acceptance task).
func ContractFirstReady(t *Task, g *Graph) error {
	if t.Type != "feature" {
		return nil
	}
	if acceptanceDone(t.ID, g, map[string]bool{}) {
		return nil
	}
	return fmt.Errorf("feature task %s has no done acceptance task among its deps (contract-first rule)", t.ID)
}

// acceptanceDone walks the dependency ancestry looking for a done acceptance task.
func acceptanceDone(id string, g *Graph, seen map[string]bool) bool {
	if seen[id] {
		return false
	}
	seen[id] = true
	t := g.task(id)
	if t == nil {
		return false
	}
	if t.Type == "acceptance" && t.Status == StatusDone {
		return true
	}
	for _, d := range t.Deps {
		if acceptanceDone(d, g, seen) {
			return true
		}
	}
	return false
}
