package qelib

import "testing"

// FuzzNormArgs: argv normalisation sees every byte a user can pass —
// it must never panic or loop.
func FuzzNormArgs(f *testing.F) {
	f.Add("--vm", "x.conf", "--format", "json")
	f.Add("-h", "", "--", "a=b")
	f.Add("\x00", "-", "--=", "‥")
	f.Fuzz(func(t *testing.T, a, b, c, d string) {
		_ = NormArgs([]string{"tool", a, b, c, d})
	})
}

// FuzzParamCheckConf: conf validation runs on arbitrary user confs —
// unknown keys, odd values and injection attempts must not panic.
func FuzzParamCheckConf(f *testing.F) {
	f.Add("ram", "4G", "cpu_cores", "4")
	f.Add("disk_img", "../../etc/passwd", "boot", "efi")
	f.Add("", "", "\x00", "a\nb")
	f.Fuzz(func(t *testing.T, k1, v1, k2, v2 string) {
		_ = ParamCheckConf(map[string]string{k1: v1, k2: v2})
	})
}
