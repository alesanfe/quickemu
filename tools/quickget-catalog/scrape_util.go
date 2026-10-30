// scrape_util.go — shared string/regex/version comparison helpers for the scrape resolvers.
package main

import (
	"regexp"
)

// matchAll returns all capture-group-1 matches of re in body.
func matchAll(body, re string) []string {
	r := regexp.MustCompile(re)
	var out []string
	for _, m := range r.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

// latestMatch returns the version-sorted-greatest match of re in body
// (equivalent of `grep -o re | sort -Vr | head -1`).
func latestMatch(body, re string) string {
	m := matchAll(body, "("+re+")")
	if len(m) == 0 {
		return ""
	}
	best := m[0]
	for _, c := range m[1:] {
		if versionLess(best, c) {
			best = c
		}
	}
	return best
}

// versionLess is a natural sort -V comparator (numeric-aware).
func versionLess(a, b string) bool {
	ta := splitNum(a)
	tb := splitNum(b)
	for i := 0; i < len(ta) && i < len(tb); i++ {
		if ta[i] == tb[i] {
			continue
		}
		na, nb := isNum(ta[i]), isNum(tb[i])
		if na && nb {
			return atoi(ta[i]) < atoi(tb[i])
		}
		return ta[i] < tb[i]
	}
	return len(ta) < len(tb)
}
func splitNum(s string) []string {
	var out []string
	cur := ""
	num := false
	for _, r := range s {
		n := r >= '0' && r <= '9'
		if n != num && cur != "" {
			out = append(out, cur)
			cur = ""
		}
		num = n
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
func isNum(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
