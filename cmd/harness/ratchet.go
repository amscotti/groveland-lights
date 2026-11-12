package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VerifyManifest checks every file listed in harness/protected.sha256 against the
// worktree copy. Lines: "<sha256>  <path>". Missing manifest file = error.
func VerifyManifest(root string) error {
	manifest := filepath.Join(root, "harness", "protected.sha256")
	b, err := os.ReadFile(manifest)
	if err != nil {
		return fmt.Errorf("checksum manifest missing: %w", err)
	}
	bad := 0
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return fmt.Errorf("malformed manifest line: %q", line)
		}
		sum, rel := parts[0], parts[1]
		got, err := fileSHA(filepath.Join(root, rel))
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: checksum: %s: %v\n", rel, err)
			bad++
			continue
		}
		if got != sum {
			fmt.Fprintf(os.Stderr, "ERROR: checksum mismatch: %s\n", rel)
			bad++
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d checksum failure(s)", bad)
	}
	return nil
}

// GenerateManifest (re)writes harness/protected.sha256 over the given path sets.
func GenerateManifest(root string) error {
	var lines []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if !manifestCovered(rel) {
			return nil
		}
		sum, err := fileSHA(p)
		if err != nil {
			return err
		}
		lines = append(lines, sum+"  "+rel)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(lines)
	out := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filepath.Join(root, "harness", "protected.sha256"), []byte(out), 0o600)
}

func manifestCovered(rel string) bool {
	return matchGlob("acceptance/**", rel) || matchGlob("testdata/**/*.golden*", rel)
}

func fileSHA(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// RatchetCompare: counts must not decrease.
func RatchetCompare(mainGo, branchGo, mainPw, branchPw int) error {
	if branchGo < mainGo {
		return fmt.Errorf("go test count ratchet: %d -> %d", mainGo, branchGo)
	}
	if branchPw < mainPw {
		return fmt.Errorf("playwright test count ratchet: %d -> %d", mainPw, branchPw)
	}
	return nil
}

// ParseVerdict returns the last VERDICT line in a review output.
func ParseVerdict(s string) (string, bool) {
	v := ""
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "VERDICT:") {
			v = strings.TrimSpace(strings.TrimPrefix(t, "VERDICT:"))
		}
	}
	if v == "PASS" || v == "BLOCK" {
		return v, true
	}
	return "", false
}

// DirtyLines filters `git status --porcelain` lines, ignoring paths that are allowed
// to be dirty in a task worktree (handoffs, runs, node_modules link, test env).
func DirtyLines(porcelain string) []string {
	var out []string
	for _, raw := range strings.Split(porcelain, "\n") {
		if len(raw) < 4 {
			continue
		}
		p := strings.TrimSpace(raw[3:])
		if p == "" {
			continue
		}
		if matchGlob("harness/handoffs/**", p) || matchGlob("harness/runs/**", p) ||
			p == "node_modules" || p == ".env.test" || p == ".merge.lock" {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// ServedModel extracts provider/model ids from a JSONL transcript (best effort).
func ServedModel(transcript string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range reModel.FindAllStringSubmatch(transcript, -1) {
		id := m[1]
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
