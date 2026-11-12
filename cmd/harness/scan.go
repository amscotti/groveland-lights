package main

import (
	"fmt"
	"regexp"
	"strings"
)

var reModel = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)

var forbiddenMarkers = []string{
	"TODO",
	"FIXME",
	`panic("not implemented")`,
}

var reNolintNoReason = regexp.MustCompile(`//nolint:?\w*\s*$`)
var reConsoleLog = regexp.MustCompile(`console\.log`)

// ScanAddedLines returns guard findings for markers in added diff lines.
// Files under acceptance/ and *_test.go are exempt (tests may reference markers).
func ScanAddedLines(added []addedLine) []string {
	var out []string
	for _, a := range added {
		if strings.HasSuffix(a.path, "_test.go") || strings.HasPrefix(a.path, "acceptance/") ||
			strings.HasPrefix(a.path, "harness/selftest/") {
			continue
		}
		for _, m := range forbiddenMarkers {
			if strings.Contains(a.line, m) {
				out = append(out, fmt.Sprintf("%s:%d: forbidden marker %q", a.path, a.n, m))
			}
		}
		if reNolintNoReason.MatchString(a.line) {
			out = append(out, fmt.Sprintf("%s:%d: //nolint without a reason", a.path, a.n))
		}
		if strings.HasPrefix(a.path, "web/static/js/") && reConsoleLog.MatchString(a.line) {
			out = append(out, fmt.Sprintf("%s:%d: console.log in web/static/js", a.path, a.n))
		}
	}
	return out
}

type addedLine struct {
	path string
	n    int
	line string
}

// ParseUnifiedAdded extracts added lines from `git diff` output (per-file).
func ParseUnifiedAdded(diff string) []addedLine {
	var out []addedLine
	var file string
	n := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			file = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if strings.HasPrefix(line, "@@") {
			// Parse the new-file start hunk offset.
			n = parseHunkStart(line)
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			out = append(out, addedLine{path: file, n: n, line: strings.TrimPrefix(line, "+")})
			n++
			continue
		}
		if strings.HasPrefix(line, "-") {
			continue
		}
		n++
	}
	return out
}

func parseHunkStart(hunk string) int {
	// @@ -a,b +c,d @@ ...  -> c
	re := regexp.MustCompile(`\+(\d+)(?:,(\d+))?`)
	m := re.FindStringSubmatch(hunk)
	if m == nil {
		return 0
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n
}

// DiffDeps compares dependency lines of go.mod/package.json between old and new
// text and returns newly added module specifiers.
func DiffDeps(oldText, newText, kind string) []string {
	extract := func(t string) map[string]bool {
		m := map[string]bool{}
		verRe := regexp.MustCompile(`^v\d+\.\d+`)
		for _, line := range strings.Split(t, "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			switch kind {
			case "go":
				// Two fields where the second looks like a version and the first
				// looks like a module path (contains a dot or slash).
				if len(f) == 2 && verRe.MatchString(f[1]) && (strings.Contains(f[0], ".") || strings.Contains(f[0], "/")) {
					m[f[0]] = true
				}
				if f[0] == "require" && len(f) >= 3 && verRe.MatchString(f[2]) {
					m[f[1]] = true
				}
			case "npm":
				if strings.HasPrefix(f[0], "\"") && len(f) >= 2 {
					m[strings.Trim(f[0], "\",")] = true
				}
			}
		}
		return m
	}
	oldD, newD := extract(oldText), extract(newText)
	var added []string
	for k := range newD {
		if !oldD[k] {
			added = append(added, k)
		}
	}
	return added
}

// DepAllowed checks an added dependency against allow_deps. Go modules match by
// major path prefix; npm by package name.
func DepAllowed(dep string, allow []string) bool {
	for _, a := range allow {
		if a == dep {
			return true
		}
		if strings.HasPrefix(dep, a) { // e.g. aws-sdk-go-v2/... allowed by aws-sdk-go-v2
			return true
		}
	}
	return false
}
