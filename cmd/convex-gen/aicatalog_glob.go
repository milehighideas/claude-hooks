package main

import (
	"regexp"
	"strings"
)

// globToRegex converts a glob ("*" within a segment, "**" any depth) to an anchored regex.
func globToRegex(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '[', ']', '{', '}', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return b.String()
}

// matchesAnyGlob reports whether path matches any of the glob patterns.
//
// Matching is CASE-INSENSITIVE: deny/forceRead policies are a safety boundary, and
// function leaf names capitalize destructive verbs (e.g. adminDeleteUser, sendBulkSMS),
// so a case-sensitive "**/*delete*" would silently miss them. Over-denying a benign
// capitalized match is acceptable; under-denying a destructive one is not.
func matchesAnyGlob(path string, patterns []string) bool {
	for _, p := range patterns {
		re, err := regexp.Compile("(?i)" + globToRegex(p))
		if err != nil {
			continue
		}
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
