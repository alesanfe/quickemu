// helpers.go: small shared bits — sibling-tool invocation, conf-file
// editing, json sugar and the /proc liveness check.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// toolOut runs a sibling quickemu-* binary and captures stdout —
// qelib.Tool resolves dev-tree and packaged layouts via FindTool.
func toolOut(name string, args ...string) (string, error) {
	bin, err := qelib.FindTool(name)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	return buf.String(), err
}

// toolRun streams a sibling's stdout through fn line by line — used
// for quickget downloads whose progress belongs in events.
func toolRun(dir, name string, fn func(string), args ...string) error {
	bin, err := qelib.FindTool(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	lbuf := make([]byte, 0, 4096)
	one := make([]byte, 1024)
	for {
		n, err := out.Read(one)
		lbuf = append(lbuf, one[:n]...)
		for {
			i := bytes.IndexByte(lbuf, '\n')
			if i < 0 {
				break
			}
			if fn != nil {
				fn(string(bytes.TrimSpace(lbuf[:i])))
			}
			lbuf = lbuf[i+1:]
		}
		if err != nil {
			break
		}
	}
	if len(lbuf) > 0 && fn != nil {
		fn(string(bytes.TrimSpace(lbuf)))
	}
	return cmd.Wait()
}

// dataFile resolves a repo data file (catalog, schemas) through the
// same layouts qelib.DataFile covers — thin wrapper so call sites
// stay readable.
func dataFile(rel ...string) string {
	return qelib.DataFile(rel...)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func trimLine(s string) string { return strings.TrimSpace(s) }

// pidAlive mirrors qelib.Alive (EPERM counts as alive; zombies don't)
// without exporting it — the seam keeps quicktest free to run a
// reaped-child fast path later.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// NewBytesReader adapts a byte slice to io.Reader for ssh stdin.
type byteReader struct{ b []byte }

func NewBytesReader(b []byte) io.Reader { return &byteReader{b: b} }

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

// confSet upserts key="value" lines in a quickemu .conf. Keys absent
// from kv are untouched; a key present with empty value is removed.
// The file is rewritten wholesale — confs are tiny.
var confKeyRe = regexp.MustCompile(`^([A-Za-z_]+)=`)

func confSet(confPath string, kv map[string]string) error {
	b, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	var keep []string
	for _, l := range strings.Split(string(b), "\n") {
		if m := confKeyRe.FindStringSubmatch(l); m != nil {
			if _, overridden := kv[m[1]]; overridden {
				continue
			}
		}
		keep = append(keep, l)
	}
	for k, v := range kv {
		if v == "" {
			continue
		}
		keep = append(keep, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	out := strings.Join(keep, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(confPath, []byte(out), 0o644)
}

// confDrop removes keys entirely — used post-install to undo the
// kernel/initrd/boot_args overrides that would re-run the installer.
func confDrop(confPath string, keys ...string) error {
	b, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	var keep []string
	for _, l := range strings.Split(string(b), "\n") {
		if m := confKeyRe.FindStringSubmatch(l); m != nil && drop[m[1]] {
			continue
		}
		keep = append(keep, l)
	}
	return os.WriteFile(confPath, []byte(strings.Join(keep, "\n")), 0o644)
}

// confGet reads one key's raw value.
func confGet(confPath, key string) string {
	b, err := os.ReadFile(confPath)
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		kv := strings.SplitN(l, "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == key {
			return strings.Trim(strings.TrimSpace(kv[1]), `"`)
		}
	}
	return ""
}

// findVMConf locates the .conf quickget produced under dir. Two
// layouts exist: flat (<dir>/<os>-<rel>.conf + <dir>/<os>-<rel>/) and
// nested (<dir>/<os>-<rel>/<os>-<rel>.conf). vmDir follows the
// launcher's own rule — the dir holding the disk image when the conf
// points into a subdirectory, else <confdir>/<vmname>.
func findVMConf(dir string) (confPath, vmDir, vmName string, err error) {
	var ents []string
	for _, pat := range []string{"*.conf", filepath.Join("*", "*.conf")} {
		g, _ := filepath.Glob(filepath.Join(dir, pat))
		ents = append(ents, g...)
	}
	if len(ents) == 0 {
		return "", "", "", fmt.Errorf("no .conf under %s", dir)
	}
	newest := ents[0]
	var mt int64
	for _, p := range ents {
		if st, e := os.Stat(p); e == nil && st.ModTime().Unix() >= mt {
			mt, newest = st.ModTime().Unix(), p
		}
	}
	confPath = newest
	vmName = strings.TrimSuffix(filepath.Base(confPath), ".conf")
	vmDir = deriveVMDir(confPath)
	return confPath, vmDir, vmName, nil
}

// deriveVMDir mirrors quickemu-launch's vmDir(): the runtime dir is
// where the disk lives — flat confs point at <name>/<image> and
// runtime files land beside the disk, not beside the conf.
func deriveVMDir(confPath string) string {
	confDir := filepath.Dir(confPath)
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	for _, k := range []string{"disk_img", "img"} {
		if d := confGet(confPath, k); d != "" {
			if filepath.IsAbs(d) {
				return filepath.Dir(d)
			}
			return filepath.Dir(filepath.Join(confDir, d))
		}
	}
	if st, err := os.Stat(filepath.Join(confDir, name)); err == nil && st.IsDir() {
		return filepath.Join(confDir, name)
	}
	return confDir
}

// resolveConfPath makes a conf-relative path absolute — conf values
// like disk_img="vm/file.img" are relative to the conf's directory.
func resolveConfPath(confPath, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(confPath), p)
}
