// suspend_test.go: QE-020 — suspend-to-disk + resume-from-state.
// The QMP channel is a scripted stub binary; the VM is a renamed
// sleep so instanceVerify/fingerprint see a live process.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// suspendStubs: quickemu-qmp answers by command (migrate→ack,
// query-migrate→completed, quit→kills the fake VM), quickemu-launch
// records its conf for resume_from assertions.
func suspendStubs(t *testing.T) {
	t.Helper()
	d := t.TempDir()
	qmp := `#!/bin/sh
case "$*" in
  *query-migrate*) echo '{"data":{"status":"completed"}}' ;;
  *migrate*)
    img=$(printf '%s' "$*" | sed -n "s/.*'\(.*\.state\.gz\)'.*/\1/p")
    [ -n "$img" ] && echo x > "$img"
    echo '{"return":{}}' ;;
  *quit*)
    if [ -n "$QE_FAKE_QMP_PID" ]; then kill "$QE_FAKE_QMP_PID" 2>/dev/null; fi
    echo '{"return":{}}' ;;
  *) echo '{"return":{}}' ;;
esac
exit 0`
	launch := `#!/bin/sh
if [ -n "$QE_LAUNCH_CONF" ]; then cat "$1" > "$QE_LAUNCH_CONF" 2>/dev/null; fi
echo "launch stub $@"
exit 0`
	for n, s := range map[string]string{
		"quickemu-qmp": qmp, "quickemu-launch": launch,
	} {
		if err := os.WriteFile(filepath.Join(d, n), []byte(s), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSuspendNotRunning(t *testing.T) {
	withConf(t, "")
	if rc := actSuspend(); rc != 1 {
		t.Fatalf("suspend on stopped VM must fail, got %d", rc)
	}
}

func TestSuspendNoQMP(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep")
	}
	vd := withConf(t, "")
	livePID(t, vd)
	if rc := actSuspend(); rc != 1 {
		t.Fatalf("suspend without QMP channel must fail, got %d", rc)
	}
}

func TestSuspendHappyPath(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep")
	}
	suspendStubs(t)
	vd := withConf(t, "guest_os=\"linux\"\n")
	pid := livePID(t, vd)
	t.Setenv("QE_FAKE_QMP_PID", itoa(pid))
	monSock(t, "-qmp.socket")
	machineTransition(stRunning, "test", "t")
	if rc := actSuspend(); rc != 0 {
		t.Fatalf("suspend rc=%d", rc)
	}
	// FSM must land in suspended; meta records the saved guest.
	if r := machineState(); r == nil || r.State != stSuspended {
		t.Fatalf("state = %+v", r)
	}
	var m suspendMetaRec
	b, err := os.ReadFile(suspendMeta())
	if err != nil || json.Unmarshal(b, &m) != nil || m.GuestOS != "linux" {
		t.Fatalf("meta: %v %q", err, b)
	}
}

func TestResumeSuspendedLaunchesIncoming(t *testing.T) {
	suspendStubs(t)
	vd := withConf(t, "guest_os=\"linux\"\n")
	img := suspendImage()
	os.WriteFile(img, []byte("fake-vmstate"), 0600)
	os.WriteFile(suspendMeta(), []byte(`{"guest_os":"linux"}`), 0600)
	machineTransition(stSuspended, "test", "t")
	if !suspendedState() {
		t.Fatal("fixture must read as suspended")
	}
	cap := filepath.Join(vd, "cap.conf")
	t.Setenv("QE_LAUNCH_CONF", cap)
	if rc := actResumeSuspended(); rc != 0 {
		t.Fatalf("resume rc=%d", rc)
	}
	b, err := os.ReadFile(cap)
	if err != nil || !strings.Contains(string(b), `resume_from="`+img+`"`) {
		t.Fatalf("launch conf missing resume_from: %q", b)
	}
	// The image is single-use — a stale copy resumed over a modified
	// disk is silent corruption.
	if _, err := os.Stat(img); !os.IsNotExist(err) {
		t.Fatal("suspend image must be consumed on successful resume")
	}
}

func TestResumeCompatible(t *testing.T) {
	vd := withConf(t, "guest_os=\"linux\"\narch=\"x86_64\"\n")
	_ = vd
	// Missing meta refuses — an unlabelled image is unverifiable.
	os.WriteFile(suspendImage(), []byte("x"), 0600)
	if err := resumeCompatible(); err == nil {
		t.Fatal("missing meta must refuse")
	}
	// Matching conf passes.
	os.WriteFile(suspendMeta(), []byte(`{"guest_os":"linux","arch":"x86_64"}`), 0600)
	if err := resumeCompatible(); err != nil {
		t.Fatalf("matching meta refused: %v", err)
	}
	// Mismatched guest refuses.
	os.WriteFile(suspendMeta(), []byte(`{"guest_os":"windows","arch":"x86_64"}`), 0600)
	if err := resumeCompatible(); err == nil {
		t.Fatal("guest_os mismatch must refuse")
	}
	// Mismatched arch refuses.
	os.WriteFile(suspendMeta(), []byte(`{"guest_os":"linux","arch":"aarch64"}`), 0600)
	if err := resumeCompatible(); err == nil {
		t.Fatal("arch mismatch must refuse")
	}
}

func TestSuspendedStateRequiresImage(t *testing.T) {
	withConf(t, "")
	machineTransition(stSuspended, "test", "t")
	if suspendedState() {
		t.Fatal("suspended record without image must not read as resumable")
	}
	os.WriteFile(suspendImage(), []byte("x"), 0600)
	if !suspendedState() {
		t.Fatal("suspended record + image must read as resumable")
	}
}

func TestShellQ(t *testing.T) {
	if got := shellQ("/a b/c"); got != `'/a b/c'` {
		t.Fatalf("got %q", got)
	}
	if got := shellQ(`/a'b`); got != `'/a'\''b'` {
		t.Fatalf("embedded quote: %q", got)
	}
}

func TestSuspendParse(t *testing.T) {
	old := actions
	actions = nil
	t.Cleanup(func() { actions = old })
	parseArgs([]string{"--suspend", "--vm", "x.conf"})
	if len(actions) != 1 || actions[0] != "vm_suspend" {
		t.Fatalf("--suspend → %v", actions)
	}
}

func TestSuspendTimeoutEnv(t *testing.T) {
	t.Setenv("QE_SUSPEND_TIMEOUT", "30")
	if d := suspendTimeout(); d != 30*time.Second {
		t.Fatalf("timeout = %v", d)
	}
}
