// quickemu-archive: ADR-0001 phase 6 + QE-090/QE-091 — manifest
// verification and safe extraction for .quickvm.tar.gz exports.
//
//	quickemu-archive verify  --archive <tgz>
//	quickemu-archive inspect --archive <tgz>
//	quickemu-archive extract --archive <tgz> --dest <dir> [--force]
//	quickemu-archive import  --archive <tgz> --dest <dir> [--force]
//	quickemu-archive recover --dest <dir>
//
// verify validates structure + manifests only. extract unpacks the
// raw member tree through staging. import is VM-aware: the archive's
// <vm>.conf + <vm>/ land as <dest>/<vm>.conf + <dest>/<vm> via a
// journalised <vm>.import.tmp staging dir and atomic renames.
// recover resolves leftover .import.tmp state (completing a fully
// staged import, cleaning a partial one). Both manifest formats are
// understood: v1 (quickvm-manifest.json + checksums) and v2
// (manifest.json, format_version 2).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.2.0"

func die(c, m string) int {
	qelib.Emit(qelib.CmdName("archive"), "quickemu-archive/"+version, false, nil,
		[]qelib.QEError{{Code: c, Message: m, Severity: "error", Retryable: false}})
	return 1
}

// ok emits the envelope; d is a pre-marshalled JSON fragment (the
// small fixed payloads built by callers below).
func ok(d string) int {
	qelib.EmitOK(qelib.CmdName("archive"), "quickemu-archive/"+version, json.RawMessage(d))
	return 0
}

// warnf writes a warning to stderr — the only channel integrity and
// trust messages may use (stdout carries the envelope/errors).
func warnf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, " - WARNING! "+format+"\n", a...)
}

func main() { os.Exit(run()) }

// args holds the parsed flag set; bool flags arrive with value "true".
type args struct {
	kv  map[string]string
	pos []string
}

// parseArgs accepts "--flag value", "--flag=value" (post-NormArgs both
// arrive split) and bare "--bool" flags.
func parseArgs(a []string, bools ...string) args {
	isBool := map[string]bool{}
	for _, b := range bools {
		isBool[b] = true
	}
	out := args{kv: map[string]string{}}
	for i := 0; i < len(a); i++ {
		s := a[i]
		if !strings.HasPrefix(s, "--") {
			out.pos = append(out.pos, s)
			continue
		}
		k := strings.TrimPrefix(s, "--")
		if isBool[k] {
			out.kv[k] = "true"
			continue
		}
		if i+1 < len(a) {
			out.kv[k] = a[i+1]
			i++
		} else {
			out.kv[k] = ""
		}
	}
	return out
}

func run() int {
	os.Args = qelib.NormArgs(os.Args)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-archive <verify|inspect|extract|import|recover> --archive <tgz> [--dest <dir>] [--force]")
		return 2
	}
	sub := os.Args[1]
	a := parseArgs(os.Args[2:], "force")
	arch := a.kv["archive"]
	if arch == "" && len(a.pos) > 0 {
		arch = a.pos[0]
	}
	dest := a.kv["dest"]
	if dest == "" {
		dest = "."
	}
	force := a.kv["force"] == "true"

	switch sub {
	case "verify":
		if arch == "" {
			fmt.Fprintln(os.Stderr, "usage: quickemu-archive verify --archive <tgz>")
			return 2
		}
		return cmdVerify(arch)
	case "inspect":
		if arch == "" {
			fmt.Fprintln(os.Stderr, "usage: quickemu-archive inspect --archive <tgz>")
			return 2
		}
		return cmdInspect(arch)
	case "extract":
		if arch == "" {
			fmt.Fprintln(os.Stderr, "usage: quickemu-archive extract --archive <tgz> --dest <dir> [--force]")
			return 2
		}
		return cmdExtract(arch, dest, force)
	case "import":
		if arch == "" {
			fmt.Fprintln(os.Stderr, "usage: quickemu-archive import --archive <tgz> --dest <dir> [--force]")
			return 2
		}
		return cmdImport(arch, dest, force)
	case "recover":
		return cmdRecover(dest)
	default:
		fmt.Fprintf(os.Stderr, "usage: quickemu-archive <verify|inspect|extract|import|recover> ... (unknown %q)\n", sub)
		return 2
	}
}

func cmdVerify(arch string) int {
	s, err := scanArchive(arch, true)
	if err != nil {
		if qe, ok := err.(*qeErr); ok {
			return die(qe.code, qe.msg)
		}
		return die("ARCHIVE_BAD", err.Error())
	}
	if err := s.verifyManifests(); err != nil {
		code := "CHECKSUM_MISMATCH"
		if i := strings.Index(err.Error(), ":"); i > 0 {
			code = err.Error()[:i]
		}
		return die(code, err.Error())
	}
	if s.v2 != nil && s.v2.IncludesTPM {
		warnf("archive carries TPM/swtpm state — secrets travel with it.")
	}
	var links []map[string]string
	for _, l := range s.links {
		kind := "symlink"
		if l.hardlink {
			kind = "hardlink"
		}
		links = append(links, map[string]string{
			"member": l.member, "target": l.target, "type": kind})
	}
	fv := 0
	vm := s.vmName
	if s.v2 != nil {
		fv = s.v2.FormatVersion
	} else if s.v1 != nil {
		fv = 1
	}
	d, _ := json.Marshal(map[string]any{
		"members": s.entries, "verified": len(s.files),
		"links": links, "format_version": fv, "vm": vm,
	})
	return ok(string(d))
}

func cmdInspect(arch string) int {
	s, err := scanArchive(arch, false)
	if err != nil {
		if qe, ok := err.(*qeErr); ok {
			return die(qe.code, qe.msg)
		}
		return die("ARCHIVE_BAD", err.Error())
	}
	data := map[string]any{
		"archive": arch, "members": s.entries, "total_bytes": s.total,
		"vm": s.vmName, "links": len(s.links),
	}
	var names []string
	for _, m := range s.members {
		names = append(names, m.name)
	}
	sort.Strings(names)
	data["member_names"] = names
	if s.v2 != nil {
		data["format_version"] = s.v2.FormatVersion
		data["created_at"] = s.v2.CreatedAt
		data["quickemu_version"] = s.v2.QuickemuVersion
		data["min_qemu_version"] = s.v2.MinQemuVersion
		data["conf"] = s.v2.Conf
		data["includes_tpm"] = s.v2.IncludesTPM
		var blobs []map[string]any
		for _, b := range s.v2.Blobs {
			blobs = append(blobs, map[string]any{
				"path": b.Path, "sha256": b.SHA256,
				"size": b.Size, "kind": b.Kind})
		}
		data["blobs"] = blobs
		var disks []map[string]any
		for _, dsk := range s.v2.Disks {
			disks = append(disks, map[string]any{
				"path": dsk.Path, "format": dsk.Format,
				"virtual_size": dsk.VirtualSize,
				"backing_file": dsk.BackingFile,
				"snapshots":    len(dsk.Snapshots)})
		}
		data["disks"] = disks
	} else if s.v1 != nil {
		data["format_version"] = 1
		data["created_at"] = s.v1.CreatedAt
	}
	d, _ := json.Marshal(data)
	return ok(string(d))
}

func cmdExtract(arch, dest string, force bool) int {
	exn, err := runExtraction(arch, dest, false, force)
	if err != nil {
		if qe, ok := err.(*qeErr); ok {
			return die(qe.code, qe.msg)
		}
		return die("EXTRACT_FAIL", err.Error())
	}
	var installed []string
	for _, rel := range exn.ex.rel {
		installed = append(installed, rel)
	}
	sort.Strings(installed)
	d, _ := json.Marshal(map[string]any{
		"dest": dest, "installed": installed, "members": exn.scan.entries,
	})
	return ok(string(d))
}

func cmdImport(arch, dest string, force bool) int {
	exn, err := runExtraction(arch, dest, true, force)
	if err != nil {
		if qe, ok := err.(*qeErr); ok {
			return die(qe.code, qe.msg)
		}
		return die("EXTRACT_FAIL", err.Error())
	}
	vm := exn.scan.vmName
	warnf("%s.conf is executed as code on launch; review it before running the VM.", vm)
	d, _ := json.Marshal(map[string]any{
		"import": vm, "dest": dest, "members": exn.scan.entries,
	})
	return ok(string(d))
}

func cmdRecover(dest string) int {
	actions, err := recoverDest(dest)
	if err != nil {
		return die("EXTRACT_FAIL", err.Error())
	}
	d, _ := json.Marshal(map[string]any{"dest": dest, "actions": actions})
	return ok(string(d))
}
