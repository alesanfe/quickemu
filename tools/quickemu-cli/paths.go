// tools/quickemu-cli/paths.go: vm dir/name/socket path helpers + sibling-tool resolver
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func tool(name string, args ...string) (string, error) {
	return toolEnv(name, nil, args...)
}

// toolEnv: tool() with extra env vars — used to pass QUICKEMU_QMP_RAW=1
// only to trusted internal flows (hotplug), never to user QMP calls.
func toolEnv(name string, env []string, args ...string) (string, error) {
	// stdout is captured for the caller; stderr passes straight through
	// to the user — Bash subprocess semantics (ERROR! lines must stay
	// on stderr for contract callers).
	return qelib.Tool(name, env, args...)
}

// vmdir: vm_identify parity — the VM state dir is dirname(disk_img)
// when the conf sets it, else <confdir>/<vmname> (the quickget
// convention of a per-VM directory beside the conf). Never just the
// conf's directory: state files must not mix into the caller's cwd.
func vmdir() string {
	confDir := filepath.Dir(vmConf)
	if d := confKV()["disk_img"]; d != "" {
		if filepath.IsAbs(d) {
			return filepath.Dir(d)
		}
		// Relative disk_img must stay inside confdir — "../x" would
		// scatter .pid/.sock/locks outside the VM tree.
		j := filepath.Join(confDir, d)
		if rel, err := filepath.Rel(confDir, j); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return filepath.Dir(j)
		}
	}
	return filepath.Join(confDir, vmname())
}

func vmname() string {
	return strings.TrimSuffix(filepath.Base(vmConf), filepath.Ext(vmConf))
}

func qmpSock() string { return qelib.SockPath(vmdir(), vmname(), qelib.SockQMP) }

func hmpSock() string { return qelib.SockPath(vmdir(), vmname(), qelib.SockMonitor) }

func pidPath() string { return filepath.Join(vmdir(), vmname()+".pid") }

// emitEnvelope: full v1 contract envelope — {schemaVersion, command,
// success, data, warnings, errors, meta{quickemuVersion,timestamp,
// operationId?}} — identical shape to qe_emit.
const qeVersion = qelib.QEVersion

func glob(pat string) []string {
	m, _ := filepath.Glob(pat)
	return m
}
