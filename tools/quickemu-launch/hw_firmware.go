// hw_firmware.go: firmware discovery, EDK2/OVMF/AAVMF pairs, EFI vars seeding
//
// Error text in this file is the Bash user-facing contract: it is
// printed verbatim to stdout, so it keeps its punctuation.
//
//lint:file-ignore ST1005 Bash parity error strings keep punctuation.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// firmwareDirs: the search roots Bash builds via SHARE_PATH
// (get_qemu_share_path). QE_FIRMWARE_DIR (nix/deb packaging) wins.
func firmwareDirs() []string {
	var dirs []string
	if d := os.Getenv("QE_FIRMWARE_DIR"); d != "" {
		dirs = append(dirs, d)
	}
	share := "/usr/share"
	if q, err := exec.LookPath("qemu-system-x86_64"); err == nil {
		if rp, err := filepath.EvalSymlinks(q); err == nil {
			share = filepath.Join(filepath.Dir(filepath.Dir(rp)), "share")
		}
	}
	for _, d := range []string{"OVMF", "edk2/ovmf", "OVMF/x64", "edk2-ovmf",
		"edk2-ovmf/x64", "edk2/x64", "qemu", "AAVMF", "edk2/aarch64"} {
		dirs = append(dirs, filepath.Join(share, d))
	}
	return dirs
}

// firmwareFile resolves the first existing firmware blob — kept for
// single-file lookups (aarch64 -bios, the plan-mode fallback).
func firmwareFile(names ...string) string {
	for _, dir := range firmwareDirs() {
		for _, n := range names {
			if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
				return filepath.Join(dir, n)
			}
		}
	}
	// Fall back to the first name under /usr/share/OVMF so the QEMU
	// error names the missing file (dry-run needs no real file).
	for _, n := range names {
		if strings.HasSuffix(n, "_CODE.fd") || strings.Contains(n, "CODE") {
			return "/usr/share/OVMF/" + n
		}
	}
	return "/usr/share/OVMF/" + names[0]
}

// firmwarePair: Bash's ovmfs=() ordered pair search — the first
// (code,vars) pair where BOTH files exist in the same directory wins.
// Returns "" when nothing resolves so callers can pick a mode or fail.
func firmwarePair(pairs [][2]string) (code, vars string) {
	for _, dir := range firmwareDirs() {
		for _, p := range pairs {
			c, v := filepath.Join(dir, p[0]), filepath.Join(dir, p[1])
			cOK := false
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				cOK = true
			}
			vOK := false
			if st, err := os.Stat(v); err == nil && !st.IsDir() {
				vOK = true
			}
			if cOK && vOK {
				return c, v
			}
		}
	}
	return "", ""
}

// isFirmwareQcow2: is_firmware_qcow2 parity — NixOS/packaged EDK2
// ships qcow2-format firmware (magic QFI\xfb); the pflash drive must
// declare format=qcow2 or QEMU treats it as raw.
func isFirmwareQcow2(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return false
	}
	return b[0] == 0x51 && b[1] == 0x46 && b[2] == 0x49 && b[3] == 0xfb
}

// x86FirmwarePairs: the Bash ovmfs list collapsed to the (code,vars)
// basename pairs present in the Go search roots.
func x86FirmwarePairs(secureboot bool) [][2]string {
	if secureboot {
		return [][2]string{
			{"OVMF_CODE_4M.secboot.fd", "OVMF_VARS_4M.ms.fd"},
			{"OVMF_CODE.secboot.fd", "OVMF_VARS.secboot.fd"},
			{"OVMF_CODE.secboot.fd", "OVMF_VARS.fd"},
			{"ovmf-x86_64-smm-ms-code.bin", "ovmf-x86_64-smm-ms-vars.bin"},
			{"edk2-x86_64-secure-code.fd", "edk2-i386-vars.fd"},
			{"OVMF_CODE.secboot.4m.fd", "OVMF_VARS.4m.fd"},
			{"OVMF_CODE_4M.secboot.qcow2", "OVMF_VARS_4M.secboot.qcow2"},
		}
	}
	return [][2]string{
		{"OVMF_CODE_4M.fd", "OVMF_VARS_4M.fd"},
		{"OVMF_CODE.fd", "OVMF_VARS.fd"},
		{"ovmf-x86_64-4m-code.bin", "ovmf-x86_64-4m-vars.bin"},
		{"edk2-x86_64-code.fd", "edk2-i386-vars.fd"},
		{"OVMF_CODE.4m.fd", "OVMF_VARS.4m.fd"},
		{"OVMF_CODE_4M.qcow2", "OVMF_VARS_4M.qcow2"},
	}
}

// a64MonolithicEDK2: the EFI_MODE=bios decision — aarch64 Windows
// guests without SecureBoot take Fedora's monolithic EDK2 via -bios
// when it is installed; anything else is a persistent pflash pair.
func a64MonolithicEDK2(c Conf, osName string) string {
	if osName != "windows" || confOn(c, "secureboot") {
		return ""
	}
	for _, dir := range firmwareDirs() {
		for _, n := range []string{"edk2/aarch64/QEMU_EFI.fd", "QEMU_EFI.fd"} {
			if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
				return filepath.Join(dir, n)
			}
		}
	}
	return ""
}

// aarch64FirmwarePairs: AAVMF/edk2 pflash pairs for ARM64 guests.
func aarch64FirmwarePairs() [][2]string {
	return [][2]string{
		{"AAVMF_CODE.fd", "AAVMF_VARS.fd"},
		{"QEMU_CODE.fd", "QEMU_VARS.fd"},
		{"QEMU_EFI-pflash.raw", "vars-template-pflash.raw"},
		{"edk2-aarch64-code.fd", "edk2-arm-vars.fd"},
	}
}

// efiVarsName: the in-vmdir vars filename — .qcow2 when the seeded
// template is qcow2 firmware (Bash writes OVMF_VARS.qcow2 then).
func efiVarsName(vmdir, template string) string {
	if template != "" && isFirmwareQcow2(template) {
		return filepath.Join(vmdir, "OVMF_VARS.qcow2")
	}
	return filepath.Join(vmdir, "OVMF_VARS.fd")
}

// seedEFIVars: configure_bios + efi_vars parity — copy the VARS
// template into the vmdir so QEMU's writable pflash unit=1 has a real
// file. Idempotent: re-seeding a live VM's vars would reset the boot
// config, so an existing file wins. Returns the vars path used.
func seedEFIVars(c Conf, template string) string {
	vmdir := c["_vmdir"]
	vars := efiVarsName(vmdir, template)
	if c["_run"] != "1" {
		return vars
	}
	// Legacy migrations (configure_bios): a prior release stored the
	// vars under the VM name or the 4M suffix — rename, don't reseed.
	if _, err := os.Stat(vars); err != nil {
		for _, old := range []string{c["_vmname"] + "-vars.fd", "OVMF_VARS_4M.fd"} {
			op := filepath.Join(vmdir, old)
			if _, err := os.Stat(op); err == nil {
				os.Rename(op, vars)
				break
			}
		}
	}
	if _, err := os.Stat(vars); err == nil {
		return vars
	}
	if template == "" {
		fmt.Fprintf(os.Stderr, "warning: no OVMF_VARS template found (looked in QE_FIRMWARE_DIR + /usr/share/OVMF*)\n")
		return vars
	}
	b, err := os.ReadFile(template)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", template, err)
		return vars
	}
	if err := os.WriteFile(vars, b, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not seed %s: %v\n", vars, err)
	}
	return vars
}

// paramCheckFirmware: the fatal half of configure_bios — macOS needs
// the quickget-downloaded OVMF + OpenCore/ESP bootloader, and the
// files are size-checked (truncated downloads produce unusable pflash).
// For generic EFI guests the seeded vars must be a writeable file.
func paramCheckFirmware(c Conf, osName, arch string) error {
	if osName == "macos" {
		vmdir := c["_vmdir"]
		code := filepath.Join(vmdir, "OVMF_CODE.fd")
		vars := ""
		for _, n := range []string{"OVMF_VARS-1024x768.fd", "OVMF_VARS-1920x1080.fd"} {
			if _, err := os.Stat(filepath.Join(vmdir, n)); err == nil {
				vars = filepath.Join(vmdir, n)
				break
			}
		}
		missing := ""
		if _, err := os.Stat(code); err != nil || vars == "" {
			missing = "Firmware"
		}
		bl := ""
		for _, n := range []string{"OpenCore.qcow2", "ESP.qcow2"} {
			if _, err := os.Stat(filepath.Join(vmdir, n)); err == nil {
				bl = filepath.Join(vmdir, n)
				break
			}
		}
		if bl == "" {
			missing = "Bootloader"
		}
		if missing != "" {
			return fmt.Errorf("ERROR! macOS %s was not found.\n       Use 'quickget' to download the required files.", missing)
		}
		if fi, err := os.Stat(code); err == nil && fi.Size() < 1048576 {
			return fmt.Errorf("ERROR! macOS firmware in %s appears to be corrupt or incomplete.\n       Delete OVMF_CODE.fd and OVMF_VARS*.fd, then re-run quickget.", vmdir)
		}
		if fi, err := os.Stat(vars); err == nil && fi.Size() < 131072 {
			return fmt.Errorf("ERROR! macOS firmware in %s appears to be corrupt or incomplete.\n       Delete OVMF_CODE.fd and OVMF_VARS*.fd, then re-run quickget.", vmdir)
		}
		return nil
	}
	// Non-EFI guests need no firmware at all (legacy BIOS).
	if !strings.Contains(resolvedBoot(c, osName), "efi") {
		return nil
	}
	// configure_bios exits when the code search resolves nothing —
	// also on --dry-run; only the vars copy is dry-run gated.
	if arch == "aarch64" {
		// Windows ARM64 with SecureBoot off prefers the monolithic
		// EDK2 via -bios (no pflash pair needed).
		if a64MonolithicEDK2(c, osName) == "" {
			if code, _ := firmwarePair(aarch64FirmwarePairs()); code == "" {
				if confOn(c, "secureboot") {
					return fmt.Errorf("ERROR! SecureBoot was requested but no SecureBoot capable firmware was found.\n       Please install OVMF firmware.")
				}
				return fmt.Errorf("ERROR! EFI boot requested but no EFI firmware found.\n       Please install OVMF firmware.")
			}
		}
	} else {
		if code, _ := firmwarePair(x86FirmwarePairs(confOn(c, "secureboot"))); code == "" {
			if confOn(c, "secureboot") {
				return fmt.Errorf("ERROR! SecureBoot was requested but no SecureBoot capable firmware was found.\n       Please install OVMF firmware.")
			}
			return fmt.Errorf("ERROR! EFI boot requested but no EFI firmware found.\n       Please install OVMF firmware.")
		}
	}
	// EFI guests: the vars file in the vmdir must be a writeable
	// regular file once it exists — Bash deletes it and asks for a
	// re-run (it is a seeded copy, re-seeding is safe).
	if c["_run"] == "1" {
		for _, name := range []string{"OVMF_VARS.fd", "OVMF_VARS.qcow2"} {
			vars := filepath.Join(c["_vmdir"], name)
			fi, err := os.Stat(vars)
			if err != nil {
				continue
			}
			f, err := os.OpenFile(vars, os.O_WRONLY, 0)
			if !fi.Mode().IsRegular() || err != nil {
				os.Remove(vars)
				return fmt.Errorf(" - EFI:      ERROR! %s is not a regular file or not writeable.\n             Deleting %s. Please re-run quickemu.", vars, vars)
			}
			f.Close()
		}
	}
	return nil
}
