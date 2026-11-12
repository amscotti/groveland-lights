package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdStatus(args []string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	g, err := loadGraph(root)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for i := range g.Tasks {
		counts[g.Tasks[i].Status]++
	}
	fmt.Printf("tasks: %d\n", len(g.Tasks))
	for _, s := range []string{StatusReady, StatusRunning, StatusReview, StatusRework, StatusMerging, StatusDone, StatusBlocked} {
		if counts[s] > 0 {
			fmt.Printf("  %-8s %d\n", s, counts[s])
		}
	}
	for i := range g.Tasks {
		t := &g.Tasks[i]
		if t.Status == StatusBlocked {
			fmt.Printf("BLOCKED %s: %s\n", t.ID, t.Findings)
		}
	}
	fmt.Println("recent progress:")
	if b, err := os.ReadFile(filepath.Join(root, "harness", "progress.log")); err == nil {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		start := 0
		if len(lines) > 15 {
			start = len(lines) - 15
		}
		for _, l := range lines[start:] {
			fmt.Println("  " + l)
		}
	}
	return nil
}

func cmdProtect(args []string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if err := GenerateManifest(root); err != nil {
		return err
	}
	fmt.Println("protect: manifest regenerated (harness/protected.sha256)")
	// Ratchet baseline for the current tree.
	bl := baseline{Go: countGoTests(root), Pw: countPwTests(root)}
	b, _ := json.Marshal(bl)
	if err := os.WriteFile(filepath.Join(root, "harness", ".ratchet.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("protect: ratchet baseline go=%d pw=%d\n", bl.Go, bl.Pw)
	return nil
}
