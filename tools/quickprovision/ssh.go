// ssh.go: guest access over the forwarded SSH port. Keys are
// ephemeral per run — a throwaway ed25519 pair injected through the
// seed, never written anywhere but the artifacts dir, and host-key
// verification is skipped because the VM's identity is by definition
// brand new on every run.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// keypair is an ephemeral ed25519 identity generated per run.
type keypair struct {
	pub      string // authorized_keys line
	signer   ssh.Signer
	privPath string // saved under the run artifacts for manual access
}

func genKeypair(artDir string) (*keypair, error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sshPub, err := ssh.NewPublicKey(pk)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(sk)
	if err != nil {
		return nil, err
	}
	k := &keypair{
		pub:    string(ssh.MarshalAuthorizedKey(sshPub)),
		signer: signer,
	}
	if artDir != "" {
		block, err := ssh.MarshalPrivateKey(sk, "")
		if err == nil {
			k.privPath = filepath.Join(artDir, "id_ed25519")
			_ = os.WriteFile(k.privPath, pem.EncodeToMemory(block), 0o600)
			_ = os.WriteFile(k.privPath+".pub",
				[]byte(k.pub), 0o644)
		}
	}
	return k, nil
}

// loadKeypair restores a run's identity from its artifacts so a
// resumed run talks SSH with the same key the guest was seeded with.
func loadKeypair(artDir string) (*keypair, error) {
	privPath := filepath.Join(artDir, "id_ed25519")
	priv, err := os.ReadFile(privPath)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pub, err := os.ReadFile(privPath + ".pub")
	if err != nil {
		return nil, err
	}
	return &keypair{pub: string(pub), signer: signer, privPath: privPath}, nil
}

// sshDial tries one SSH connection. InsecureIgnoreHostKey is
// deliberate: the guest was created seconds ago and has no prior
// identity to pin against.
func sshDial(port int, user string, signer ssh.Signer) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	return ssh.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), cfg)
}

// sshExec runs cmd over a fresh session and returns combined output
// and the remote exit status.
func sshExec(c *ssh.Client, cmd string) (string, int, error) {
	s, err := c.NewSession()
	if err != nil {
		return "", -1, err
	}
	defer s.Close()
	out, err := s.CombinedOutput(cmd)
	rc := 0
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			rc = ee.ExitStatus()
		} else {
			return string(out), -1, err
		}
	}
	return string(out), rc, nil
}

var errSSHDeadline = errors.New("ssh: command deadline exceeded")

// sshExecDeadline runs cmd like sshExec but abandons the session once
// d elapses — unbounded remote commands must never wedge a run (an
// unprivileged `cloud-init status --wait` spins forever on guests
// keeping /run/cloud-init root-only). The orphaned remote process is
// left to the guest's init.
func sshExecDeadline(c *ssh.Client, cmd string, d time.Duration) (string, int, error) {
	s, err := c.NewSession()
	if err != nil {
		return "", -1, err
	}
	type res struct {
		out string
		rc  int
		err error
	}
	done := make(chan res, 1)
	go func() {
		out, err := s.CombinedOutput(cmd)
		rc := 0
		if err != nil {
			if ee, ok := err.(*ssh.ExitError); ok {
				rc = ee.ExitStatus()
			} else {
				done <- res{string(out), -1, err}
				return
			}
		}
		done <- res{string(out), rc, nil}
	}()
	select {
	case r := <-done:
		s.Close()
		return r.out, r.rc, r.err
	case <-time.After(d):
		s.Close()
		return "", -1, errSSHDeadline
	}
}

// sshWrite drops content at a remote path via stdin (no scp/sftp
// dependency — the guest only needs a shell).
func sshWrite(c *ssh.Client, dest, mode string, content []byte) error {
	s, err := c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin = NewBytesReader(content)
	return s.Run(fmt.Sprintf(
		"sudo tee %s >/dev/null && sudo chmod %s %s",
		shellQ(dest), shellQ(mode), shellQ(dest)))
}

// sshWriteWin drops content at a remote Windows path via a base64
// PowerShell write — cmd.exe has no tee and SFTP is not guaranteed on
// a fresh OpenSSH.Server install. Mode/owner don't translate to NTFS
// ACLs; the caller logs that they're ignored.
func sshWriteWin(c *ssh.Client, dest string, content []byte) error {
	b64 := base64.StdEncoding.EncodeToString(content)
	out, rc, err := sshExec(c,
		`powershell -NoProfile -Command "[IO.File]::WriteAllBytes('`+psq(dest)+
			`',[Convert]::FromBase64String('`+b64+`'))"`)
	if err != nil {
		return err
	}
	if rc != 0 {
		return fmt.Errorf("rc=%d: %s", rc, tailLines(out, 3))
	}
	return nil
}

// psq escapes a single-quoted PowerShell literal (quotes double).
func psq(s string) string { return strings.ReplaceAll(s, "'", "''") }

// shellQ single-quotes a string for /bin/sh.
func shellQ(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
		} else {
			out += string(r)
		}
	}
	return out + "'"
}
