// uname.go — `uname -s -r -m` fields without spawning uname: the
// kernel info comes from the uname(2) syscall (x/sys/unix).
package qelib

import (
	"strings"

	"golang.org/x/sys/unix"
)

// UnameFields renders the requested uname flags ("-s -r -m") from the
// uname(2) syscall; unknown/empty fields are skipped.
func UnameFields(flags string) string {
	var u unix.Utsname
	if unix.Uname(&u) != nil {
		return ""
	}
	m := map[byte]string{
		's': unix.ByteSliceToString(u.Sysname[:]),
		'r': unix.ByteSliceToString(u.Release[:]),
		'v': unix.ByteSliceToString(u.Version[:]),
		'm': unix.ByteSliceToString(u.Machine[:]),
		'n': unix.ByteSliceToString(u.Nodename[:]),
	}
	var out []string
	for _, f := range strings.Fields(flags) {
		for _, c := range []byte(strings.TrimPrefix(f, "-")) {
			if v, ok := m[c]; ok && v != "" {
				out = append(out, v)
			}
		}
	}
	return strings.Join(out, " ")
}
