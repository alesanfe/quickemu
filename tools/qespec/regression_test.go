package qespec

import (
	"testing"
)

// ---- spec/regression/quickgetc_regressions_spec.sh ----
// list_csv column order matches header; unknown OS exits non-zero.
// QUICKGETC_CATALOG override keeps it hermetic.

func TestQuickgetcRegressions(t *testing.T) {
	dir := t.TempDir()
	cat := Write(t, dir, "cat.json",
		`[{"name":"zzz","pretty_name":"ZZZ OS","releases":[{"release":"1","edition":null,"arch":"x86_64","url":"http://127.0.0.1/x.iso"}]}]`)

	t.Run("list_csv column order", func(t *testing.T) {
		Run(t, "quickgetc",
			[]RunOpt{Env("QUICKGETC_CATALOG=" + cat)},
			"--list-csv").Contains(t, "ZZZ OS,zzz,1")
	})
	t.Run("unknown OS fails", func(t *testing.T) {
		r := Run(t, "quickgetc",
			[]RunOpt{Env("QUICKGETC_CATALOG=" + cat)},
			"not-a-real-os", "1")
		if r.OK() {
			t.Error("unknown OS exited 0")
		}
		r.Contains(t, "not a supported OS")
	})
}
