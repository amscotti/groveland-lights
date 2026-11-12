package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Policy is harness/policy.json. Only bootstrap and humans change it.
type Policy struct {
	Models           map[string]string `json:"models"` // logical name -> provider/model id
	Roles            map[string]string `json:"roles"`  // agent file name -> logical model name
	EscalateWorker   string            `json:"escalate_worker_to"`
	EscalateUIWorker string            `json:"escalate_ui_worker_to"`
	VisionRoles      []string          `json:"vision_roles"`
	VisionModels     []string          `json:"vision_models"`
	MaxParallel      int               `json:"max_parallel"`
	MaxAttempts      int               `json:"max_attempts"`
	CoverageFloor    float64           `json:"coverage_floor"`
	CoveragePackages []string          `json:"coverage_packages"`
	Protected        []string          `json:"protected"`
	HoldoutDir       string            `json:"holdout_dir"`
}

// Task statuses. Only the scheduler writes these.
const (
	StatusReady   = "ready"
	StatusRunning = "running"
	StatusReview  = "review"
	StatusRework  = "rework"
	StatusMerging = "merging"
	StatusDone    = "done"
	StatusBlocked = "blocked"
)

type Budget struct {
	Steps   int `json:"steps"`
	WallMin int `json:"wall_min"`
}

type Task struct {
	ID        string   `json:"id"`
	Milestone string   `json:"milestone"`
	Type      string   `json:"type"` // feature | acceptance | fix | chore | infra
	Role      string   `json:"role"` // worker | ui-worker | test-author | integrator
	Title     string   `json:"title"`
	Deps      []string `json:"deps"`
	Owns      []string `json:"owns"`
	Claims    []string `json:"claims"`
	UI        bool     `json:"ui"`
	AllowDeps []string `json:"allow_deps"`
	Status    string   `json:"status"`
	Attempts  int      `json:"attempts"`
	Model     string   `json:"model,omitempty"` // escalation override (logical name)
	Budget    Budget   `json:"budget"`
	Findings  string   `json:"findings,omitempty"` // appended by review BLOCKs
}

type Milestone struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Assertions []string `json:"assertions"`
}

type Graph struct {
	Milestones []Milestone `json:"milestones"`
	Tasks      []Task      `json:"tasks"`
}

// Assertion is one entry in acceptance/contract.json.
type Assertion struct {
	ID       string `json:"id"`
	When     string `json:"when"`
	Shall    string `json:"shall"`
	Gate     *bool  `json:"gate,omitempty"`     // nil/true = gating; false = recorded target
	Verified string `json:"verified,omitempty"` // "human" = field check
	Visual   string `json:"visual,omitempty"`   // visual notes for the vision reviewer
}

func loadPolicy(root string) (Policy, error) {
	var p Policy
	b, err := os.ReadFile(filepath.Join(root, "harness", "policy.json"))
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("policy.json: %w", err)
	}
	if p.MaxParallel == 0 {
		p.MaxParallel = 2
	}
	if p.MaxAttempts == 0 {
		p.MaxAttempts = 3
	}
	return p, nil
}

func loadGraph(root string) (Graph, error) {
	var g Graph
	b, err := os.ReadFile(filepath.Join(root, "harness", "tasks", "graph.json"))
	if err != nil {
		return g, err
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return g, fmt.Errorf("graph.json: %w", err)
	}
	return g, nil
}

func saveGraph(root string, g Graph) error {
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(filepath.Join(root, "harness", "tasks", "graph.json"), b, 0o644)
}

func loadAssertions(root string) ([]Assertion, error) {
	var a struct {
		Assertions []Assertion `json:"assertions"`
	}
	b, err := os.ReadFile(filepath.Join(root, "acceptance", "contract.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("contract.json: %w", err)
	}
	return a.Assertions, nil
}

func (g *Graph) task(id string) *Task {
	for i := range g.Tasks {
		if g.Tasks[i].ID == id {
			return &g.Tasks[i]
		}
	}
	return nil
}

func (t *Task) modelLogical(p Policy) string {
	if t.Model != "" {
		return t.Model
	}
	return p.Roles[t.Role]
}
