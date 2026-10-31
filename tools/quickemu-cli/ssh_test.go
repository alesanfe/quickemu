// ssh_test.go: QE-030–033 — ssh/scp/sftp/ssh-config/ssh-forget.
// Resolution is tested against .ports records and conf keys; exec
// goes through the execCommand seam (stubs capture argv).
package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sshFixture: a temp VM dir + conf + globals, restored on cleanup.
// Returns vmdir() — <confdir>/vm per the quickget convention.
func sshFixture(t *testing.T, confBody string) (dir string) {
	t.Helper()
	confDir := t.TempDir()
	conf := filepath.Join(confDir, "vm.conf")
	if err := os.WriteFile(conf, []byte(confBody), 0644); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(confDir, "vm") // vmdir(): <confdir>/<vmname>
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	oldConf, oldFmt := vmConf, outFmt
	oldState := sshVerbState
	vmConf, outFmt = conf, "text"
	sshVerbState = struct {
		user     string
		port     string
		jump     string
		agent    bool
		verbose  bool
		ansible  bool
		alias    string
		pos      []string
		passthru []string
	}{}
	t.Cleanup(func() { vmConf, outFmt, sshVerbState = oldConf, oldFmt, oldState })
	return dir
}

func writePorts(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "vm.ports"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSSHResolvePortsRecord(t *testing.T) {
	dir := sshFixture(t, "guest_os=linux\nssh_user=debian\n")
	writePorts(t, dir, "ssh,22221,127.0.0.1\nmonitor,4440,\n")
	tg, err := sshResolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tg.port != "22221" || tg.host != "127.0.0.1" || tg.user != "debian" {
		t.Fatalf("got %+v", tg)
	}
	if tg.alias != "quickemu-vm" {
		t.Fatalf("alias = %q", tg.alias)
	}
}

func TestSSHResolveNonLoopbackBind(t *testing.T) {
	dir := sshFixture(t, "guest_os=linux\n")
	writePorts(t, dir, "ssh,22220,192.168.1.5\n")
	tg, err := sshResolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tg.host != "192.168.1.5" {
		t.Fatalf("bind addr should win over loopback, got %q", tg.host)
	}
}

func TestSSHResolveWildcardBindIsLoopback(t *testing.T) {
	dir := sshFixture(t, "")
	writePorts(t, dir, "ssh,22222,0.0.0.0\n")
	tg, err := sshResolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tg.host != "127.0.0.1" {
		t.Fatalf("0.0.0.0 bind must connect via loopback, got %q", tg.host)
	}
}

func TestSSHResolveConfPortFallback(t *testing.T) {
	sshFixture(t, "ssh_port=22225\nssh_user=ubuntu\n")
	tg, err := sshResolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tg.port != "22225" || tg.host != "127.0.0.1" {
		t.Fatalf("got %+v", tg)
	}
}

func TestSSHResolveFlagWins(t *testing.T) {
	dir := sshFixture(t, "ssh_port=22225\n")
	writePorts(t, dir, "ssh,22221,127.0.0.1\n")
	sshVerbState.port = "22229"
	tg, err := sshResolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tg.port != "22229" {
		t.Fatalf("flag port must win, got %q", tg.port)
	}
}

func TestSSHResolveBadFlagPort(t *testing.T) {
	sshFixture(t, "")
	sshVerbState.port = "-oProxyCommand=evil" // option-shaped injection
	if _, err := sshResolve(); err == nil {
		t.Fatal("invalid port must fail, not land in argv")
	}
}

func TestSSHKnownHostsHardening(t *testing.T) {
	dir := sshFixture(t, "")
	tg := &sshTarget{kh: filepath.Join(dir, "vm.ssh_known_hosts")}
	if err := sshKnownHosts(tg); err != nil {
		t.Fatalf("create: %v", err)
	}
	fi, _ := os.Stat(tg.kh)
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("known_hosts perm = %o, want 0600", fi.Mode().Perm())
	}
	// Loose perms get tightened on next use.
	os.Chmod(tg.kh, 0644)
	if err := sshKnownHosts(tg); err != nil {
		t.Fatalf("tighten: %v", err)
	}
	fi, _ = os.Stat(tg.kh)
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("perm not tightened: %o", fi.Mode().Perm())
	}
	// A symlink is refused — trust writes must not escape the VM dir.
	os.Remove(tg.kh)
	target := filepath.Join(dir, "elsewhere")
	os.WriteFile(target, []byte{}, 0644)
	os.Symlink(target, tg.kh)
	if err := sshKnownHosts(tg); err == nil {
		t.Fatal("symlink known_hosts must be refused")
	}
}

// stubExec records the argv the ssh-family runner builds.
func stubExec(t *testing.T) *[]string {
	t.Helper()
	got := &[]string{}
	old := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		*got = append(*got, name+" "+strings.Join(args, " "))
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = old })
	return got
}

func TestActSSHArgv(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("no ssh client on this host")
	}
	dir := sshFixture(t, "ssh_user=debian\n")
	writePorts(t, dir, "ssh,22220,127.0.0.1\n")
	sshVerbState.agent = true
	sshVerbState.jump = "bastion"
	got := stubExec(t)
	if rc := actSSH(); rc != 0 {
		t.Fatalf("actSSH rc=%d", rc)
	}
	joined := strings.Join(*got, "\n")
	for _, want := range []string{
		"ssh -p 22220",
		"UserKnownHostsFile=" + filepath.Join(dir, "vm.ssh_known_hosts"),
		"HostKeyAlias=quickemu-vm",
		"StrictHostKeyChecking=accept-new",
		"-A", "-J bastion", "debian@127.0.0.1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv missing %q:\n%s", want, joined)
		}
	}
}

func TestActSCPPushReshape(t *testing.T) {
	if _, err := exec.LookPath("scp"); err != nil {
		t.Skip("no scp client on this host")
	}
	dir := sshFixture(t, "")
	writePorts(t, dir, "ssh,22220,127.0.0.1\n")
	local := filepath.Join(dir, "file.txt")
	os.WriteFile(local, []byte("x"), 0644)
	sshVerbState.pos = []string{local, "/home/user/"}
	got := stubExec(t)
	if rc := actSCP(); rc != 0 {
		t.Fatalf("actSCP rc=%d", rc)
	}
	joined := strings.Join(*got, "\n")
	if !strings.Contains(joined, "-P 22220") {
		t.Fatalf("scp uses -P not -p:\n%s", joined)
	}
	if !strings.Contains(joined, "@"+"127.0.0.1:/home/user/") {
		t.Fatalf("bare remote path must be re-shaped:\n%s", joined)
	}
	if strings.Contains(joined, "127.0.0.1:"+local) {
		t.Fatalf("local src must NOT be re-shaped:\n%s", joined)
	}
}

func TestActSCPPullReshape(t *testing.T) {
	if _, err := exec.LookPath("scp"); err != nil {
		t.Skip("no scp client on this host")
	}
	dir := sshFixture(t, "")
	writePorts(t, dir, "ssh,22220,127.0.0.1\n")
	// ':' marker = unambiguous remote src (a same-named host path
	// can't fool the heuristic).
	sshVerbState.pos = []string{":/var/log/syslog", dir}
	got := stubExec(t)
	if rc := actSCP(); rc != 0 {
		t.Fatalf("actSCP rc=%d", rc)
	}
	joined := strings.Join(*got, "\n")
	if !strings.Contains(joined, "127.0.0.1:/var/log/syslog") {
		t.Fatalf("pull: src must be remote:\n%s", joined)
	}
}

func TestActSSHConfig(t *testing.T) {
	dir := sshFixture(t, "ssh_user=ubuntu\n")
	writePorts(t, dir, "ssh,22223,127.0.0.1\n")
	got := stubExec(t) // ssh-config must not exec anything
	if rc := actSSHConfig(); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if len(*got) != 0 {
		t.Fatalf("ssh-config spawned a process: %v", *got)
	}
}

func TestActSSHForgetNoFile(t *testing.T) {
	sshFixture(t, "")
	if rc := actSSHForget(); rc != 0 {
		t.Fatalf("forget with no known_hosts must succeed, rc=%d", rc)
	}
}

func TestActSSHForgetManualFilter(t *testing.T) {
	dir := sshFixture(t, "")
	kh := filepath.Join(dir, "vm.ssh_known_hosts")
	os.WriteFile(kh, []byte(
		"quickemu-vm ssh-ed25519 AAAA\n"+
			"quickemu-other ssh-ed25519 BBBB\n"), 0600)
	old := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false") // force the manual-filter path
	}
	t.Cleanup(func() { execCommand = old })
	if rc := actSSHForget(); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	b, _ := os.ReadFile(kh)
	if strings.Contains(string(b), "quickemu-vm ") {
		t.Fatal("alias line must be dropped")
	}
	if !strings.Contains(string(b), "quickemu-other") {
		t.Fatal("other VMs' keys must survive")
	}
}

func TestSSHRunExitCode(t *testing.T) {
	old := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 42")
	}
	t.Cleanup(func() { execCommand = old })
	if _, err := exec.LookPath("ssh"); err == nil {
		if rc := sshRun("ssh", nil); rc != 42 {
			t.Fatalf("exit code must propagate, got %d", rc)
		}
	}
}

func TestSSHRunMissingBinary(t *testing.T) {
	if rc := sshRun("definitely-not-an-ssh-binary-xyz", nil); rc != 1 {
		t.Fatalf("missing binary must fail 1, got %d", rc)
	}
}

func TestSSHInterceptVerbs(t *testing.T) {
	old := actions
	actions = nil
	t.Cleanup(func() { actions = old })
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"ssh"}, "vm_ssh"},
		{[]string{"scp", "a", "b"}, "vm_scp"},
		{[]string{"sftp"}, "vm_sftp"},
		{[]string{"ssh-config"}, "vm_ssh_config"},
		{[]string{"ssh-forget"}, "vm_ssh_forget"},
	} {
		actions = nil
		if !sshIntercept(tc.args) {
			t.Fatalf("%v not intercepted", tc.args)
		}
		if len(actions) != 1 || actions[0] != tc.want {
			t.Fatalf("%v → actions %v, want [%s]", tc.args, actions, tc.want)
		}
	}
}

func TestSSHInterceptValueTokens(t *testing.T) {
	// "--type scp" is a flag VALUE, not the subcommand.
	if sshIntercept([]string{"--type", "scp", "--vm", "x"}) {
		t.Fatal("scp as a flag value must not be intercepted")
	}
	if sshIntercept([]string{"--status"}) {
		t.Fatal("unrelated argv must pass through")
	}
}

func TestSSHParseUserAndJump(t *testing.T) {
	old := actions
	actions = nil
	t.Cleanup(func() { actions = old })
	sshFixture(t, "")
	sshIntercept([]string{"ssh", "-l", "admin", "-J", "jump@host", "-A"})
	if sshVerbState.user != "admin" || sshVerbState.jump != "jump@host" || !sshVerbState.agent {
		t.Fatalf("state %+v", sshVerbState)
	}
}

func TestSSHRemoteReshape(t *testing.T) {
	tg := &sshTarget{user: "u", host: "h"}
	if got := tg.sshRemote("host:already", true); got != "host:already" {
		t.Fatalf("explicit target must pass through, got %q", got)
	}
	if got := tg.sshRemote("/bare", false); got != "/bare" {
		t.Fatalf("local side must pass through, got %q", got)
	}
	if got := tg.sshRemote("/bare", true); got != "u@h:/bare" {
		t.Fatalf("got %q", got)
	}
}

func TestSSHPortValid(t *testing.T) {
	for p, ok := range map[string]bool{
		"22": true, "65535": true, "1": true,
		"0": false, "65536": false, "-1": false,
		"abc": false, "-oProxy": false, "": false,
	} {
		if sshPortValid(p) != ok {
			t.Fatalf("sshPortValid(%q) = %v, want %v", p, !ok, ok)
		}
	}
}

func TestSSHResolveErrNoForwardNoAgent(t *testing.T) {
	sshFixture(t, "") // no .ports, no ssh_port, no agent → error
	// guestDial will fail (no socket) — must surface a usable error.
	_, err := sshResolve()
	if err == nil {
		t.Fatal("no forward + no agent must error, not default silently")
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && strings.Contains(pathErr.Path, "known_hosts") {
		t.Fatalf("unexpected error shape: %v", err)
	}
}
