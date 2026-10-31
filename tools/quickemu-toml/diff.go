// diff.go: minimal unified diff (context=3) for `config migrate` —
// LCS over lines; .conf/.toml files are a few hundred lines at most,
// so the O(n·m) table stays tiny. Over the cap we degrade to a plain
// old/new dump rather than balloon memory.
package main

import (
	"fmt"
	"strings"
)

const diffMaxLines = 2000

type diffOp struct {
	kind byte // ' ' ctx, '-' del, '+' add
	text string
}

// lcsDiff computes the line-level edit script old -> new.
func lcsDiff(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// lcs[i][j] = LCS length of a[i:], b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}

func splitLines(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// unifiedDiff renders a `diff -u3`-style patch between old and new
// content labelled `name` ("a/name"/"b/name" headers like git).
func unifiedDiff(name string, old, new []byte) string {
	a, b := splitLines(old), splitLines(new)
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", name, name)
	if len(a) > diffMaxLines || len(b) > diffMaxLines {
		fmt.Fprintf(&out, "@@ files differ (%d -> %d lines; too large for hunks) @@\n", len(a), len(b))
		for _, l := range a {
			out.WriteString("-" + l + "\n")
		}
		for _, l := range b {
			out.WriteString("+" + l + "\n")
		}
		return out.String()
	}
	ops := lcsDiff(a, b)
	// Emit hunks: runs of changes with 3 lines of context merged when
	// their context windows overlap.
	const ctx = 3
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := i - ctx
		if start < 0 {
			start = 0
		}
		end := i
		// extend the hunk while changes sit within ctx of each other
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			// count the run of context
			j := end
			for j < len(ops) && ops[j].kind == ' ' {
				j++
			}
			if j >= len(ops) || j-end > 2*ctx {
				break
			}
			end = j
		}
		if end+ctx < len(ops) {
			end += ctx
		} else {
			end = len(ops)
		}
		var oldN, newN int
		for _, o := range ops[:start] {
			if o.kind != '+' {
				oldN++
			}
			if o.kind != '-' {
				newN++
			}
		}
		hOldN, hNewN := oldN+1, newN+1
		var oldCnt, newCnt int
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				oldCnt++
			}
			if o.kind != '-' {
				newCnt++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", hOldN, oldCnt, hNewN, newCnt)
		for _, o := range ops[start:end] {
			out.WriteByte(o.kind)
			out.WriteString(o.text)
			out.WriteByte('\n')
		}
		i = end
	}
	return out.String()
}
