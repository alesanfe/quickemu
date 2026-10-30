// vmcreate.go — .conf generation extracted from main.go: createVM
// resolves the image set, makeVMConfig renders, createConfig writes.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func createVM(e *OSEntry, entry *Release, vmPath string) int {
	images := map[string]string{}
	for _, t := range imageTypes {
		srcs := entry.sourcesFor(t)
		if len(srcs) == 0 {
			continue
		}
		s := srcs[0]
		switch {
		case s.Web != nil:
			final := handleWebSource(t, s, vmPath)
			if final == "" {
				return 1
			}
			images[t] = final
		case s.FileName != "":
			if !confSafe(s.FileName) {
				fmt.Printf("ERROR! Unsafe file_name in catalog: %q\n", s.FileName)
				return 1
			}
			images[t] = vmPath + "/" + s.FileName
		case s.Custom != "":
			fmt.Println("ERROR! Custom source not implemented")
			return 1
		default:
			fmt.Printf("ERROR! No source found for %s\n", t)
			return 1
		}
	}
	if operation == "download" {
		return 0
	}
	return makeVMConfig(e, entry, vmPath, images)
}

// handleWebSource: download + verify + decompress a web source.
func handleWebSource(imageType string, s Source, vmPath string) string {
	url := s.Web.URL
	fileName := s.Web.FileName
	if fileName == "" {
		fileName = filepath.Base(url)
	}
	if !confSafe(fileName) {
		fmt.Printf("ERROR! Unsafe file_name in catalog: %q\n", fileName)
		return ""
	}
	if webGet(url, vmPath, fileName, nil) != 0 {
		return ""
	}
	if s.Web.Checksum != "" {
		if !checkHash(filepath.Join(vmPath, fileName), s.Web.Checksum) {
			return ""
		}
	}
	final := filepath.Join(vmPath, fileName)
	if operation == "download" {
		final = fileName
		vmPath = "."
	}
	return extractArchive(s.Web.ArchiveFormat, final, vmPath, imageType)
}

// makeVMConfig: write <vmpath>.conf with guest_os/disk_img/arch_vm/
// ram/image keys.
func makeVMConfig(e *OSEntry, entry *Release, vmPath string, images map[string]string) int {
	conf := vmPath + ".conf"
	if _, err := os.Stat(conf); err == nil {
		return 0
	}
	guest := entry.GuestOS
	if guest == "" {
		guest = e.GuestOS
	}
	if guest == "" {
		guest = "linux"
	}
	// Every catalog-derived string lands inside a quoted conf line —
	// a field carrying quotes or control bytes would inject entries.
	for _, v := range []string{guest, entry.Arch, vmPath} {
		if !confSafe(v) {
			fmt.Printf("ERROR! Unsafe catalog value for conf: %q\n", v)
			return 1
		}
	}
	for _, v := range images {
		if !confSafe(v) {
			fmt.Printf("ERROR! Unsafe image path for conf: %q\n", v)
			return 1
		}
	}
	qe, _ := exec.LookPath("quickemu")
	if qe == "" {
		qe = "quickemu"
	}
	var b strings.Builder
	b.WriteString("#!" + qe + " --vm\n")
	b.WriteString("guest_os=\"" + guest + "\"\n")
	b.WriteString("disk_img=\"" + vmPath + "/disk.qcow2\"\n")
	b.WriteString("arch_vm=\"" + entry.Arch + "\"\n")
	fmt.Println("Making " + conf)
	for _, t := range imageTypes {
		if v, ok := images[t]; ok {
			b.WriteString(t + "='" + v + "'\n")
		}
	}
	if entry.RAM > 0 {
		fmt.Fprintf(&b, "ram=%d\n", entry.RAM/1024/1024/1024)
	}
	if err := os.WriteFile(conf, []byte(b.String()), 0755); err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Println(" - Setting " + conf + " executable")
	return 0
}

// createConfig: --create-config <vm-path> <path|url> — simple image
// move + minimal conf (the quickgetc variant, not quickget's).
func createConfig(vmPath, input string) int {
	if vmPath == "" || input == "" {
		fmt.Println("ERROR! --create-config requires a VM name and a path or URL to an image.")
		return 1
	}
	if err := os.Mkdir(vmPath, 0755); err != nil {
		fmt.Printf("ERROR! Could not create directory: %s. Please verify that it does not already exist.\n", vmPath)
		return 1
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		input = webRedirect(input)
		low := strings.ToLower(input)
		if !strings.HasSuffix(low, ".iso") && !strings.HasSuffix(low, ".img") && !strings.HasSuffix(low, ".qcow2") {
			fmt.Println("ERROR! Only ISO, IMG and QCOW2 file types are supported for --create-config")
			return 1
		}
		if webGet(input, vmPath, filepath.Base(input), nil) != 0 {
			return 1
		}
		input = filepath.Join(vmPath, filepath.Base(input))
	}
	if _, err := os.Stat(input); err != nil {
		fmt.Println("ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file.")
		return 1
	}
	low := strings.ToLower(input)
	imageKey := ""
	switch {
	case strings.HasSuffix(low, ".iso"):
		imageKey = "iso"
	case strings.HasSuffix(low, ".img"):
		imageKey = "img"
	case strings.HasSuffix(low, ".qcow2"):
		imageKey = "disk_img"
	default:
		fmt.Println("ERROR! Only ISO, IMG and QCOW2 file types are supported for --create-config")
		return 1
	}
	base := filepath.Base(input)
	if filepath.Dir(input) != vmPath {
		if err := qelib.MoveFile(input, filepath.Join(vmPath, base)); err != nil {
			fmt.Println(err)
			return 1
		}
	}
	guest := "linux"
	switch {
	case strings.Contains(low, "windows"), strings.Contains(low, "win"):
		guest = "windows"
	case strings.Contains(low, "macos"):
		guest = "macos"
	case strings.Contains(low, "bsd"):
		guest = "freebsd"
	}
	fmt.Printf("Creating %s.conf for %s (guest_os=%s).\n", vmPath, base, guest)
	qe, _ := exec.LookPath("quickemu")
	if qe == "" {
		qe = "quickemu"
	}
	conf := "#!" + qe + " --vm\n" +
		"guest_os=\"" + guest + "\"\n" +
		imageKey + "=\"" + vmPath + "/" + base + "\"\n"
	if err := os.WriteFile(vmPath+".conf", []byte(conf), 0755); err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Printf("\nTo start your virtual machine run:\n    quickemu --vm %s.conf\n", vmPath)
	return 0
}
