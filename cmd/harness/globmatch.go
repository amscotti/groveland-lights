package main

import (
	"path"
	"regexp"
	"strings"
)

// matchGlob matches repo-relative paths against ownership/protected globs.
// Semantics: '**' matches anything including '/', '*' matches anything except '/',
// everything else is literal. A pattern with no '/' matches the file name anywhere
// only when the pattern starts with '**/'.
func matchGlob(pattern, p string) bool {
	p = path.Clean("/" + p)[1:] // normalize without leading ./
	if pattern == "**" || pattern == "*" {
		return true
	}
	re := globToRegexp(pattern)
	return re.MatchString(p)
}

func globToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				for i+1 < len(pattern) && pattern[i+1] == '*' {
					i++
				}
				// "**/" also matches zero directories: "a/**/b" == "a/b".
				if strings.HasPrefix(pattern[i:], "**/") {
					b.WriteString("(?:.*/)?")
					i += 3
					continue
				}
				b.WriteString(".*")
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
		i++
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// ownsCovered reports whether p is covered by any of the owns globs.
func ownsCovered(owns []string, p string) bool {
	for _, o := range owns {
		if matchGlob(o, p) {
			return true
		}
	}
	return false
}

// protectedMatch reports which protected glob p matches, if any.
func protectedMatch(protected []string, p string) (string, bool) {
	for _, pat := range protected {
		if matchGlob(pat, p) {
			return pat, true
		}
	}
	return "", false
}

// ownsOverlap reports whether any glob pair overlaps. Two globs overlap if there
// exists a plausible path matching both; we approximate by checking cross-matches of
// each pattern against paths synthesized from the other pattern (its literal parts).
func ownsOverlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if globsOverlap(x, y) {
				return true
			}
		}
	}
	return false
}

func globsOverlap(x, y string) bool {
	if x == "**" || y == "**" || x == y {
		return true
	}
	// Literal skeletons: strip wildcards to get representative paths, then cross-match.
	sx, sy := skeleton(x), skeleton(y)
	if sx == "" || sy == "" {
		return true // wildcards at the root: assume overlap, be safe
	}
	return matchGlob(x, sy) || matchGlob(y, sx)
}

// skeleton turns "internal/img/**" into "internal/img" (leading literal prefix).
func skeleton(pattern string) string {
	if i := strings.IndexAny(pattern, "*?"); i >= 0 {
		pattern = pattern[:i]
	}
	return strings.Trim(pattern, "/")
}
