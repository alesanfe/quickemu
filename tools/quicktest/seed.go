// seed.go: NoCloud seed assembly. A cidata seed is just an ISO9660
// (volid "cidata", Rock Ridge + Joliet) holding user-data/meta-data —
// the same pure-Go diskfs path quickget-vm's seed.go uses, replacing
// the cloud-localds > genisoimage > xorrisofs external chain.
package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	iso9660fs "github.com/diskfs/go-diskfs/filesystem/iso9660"
)

// writeSeedISO builds a cidata ISO in dir from files keyed by their
// in-ISO path ("/user-data" → local file). Deterministic order keeps
// builds reproducible.
func writeSeedISO(out string, files map[string]string) error {
	var total int64 = 1 << 20
	for _, src := range files {
		if st, err := os.Stat(src); err == nil {
			total += st.Size() + 8192
		}
	}
	// diskfs.Create refuses to overwrite — a rebuilt seed is expected.
	if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
		return err
	}
	d, err := diskfs.Create(out, total*2, diskfs.SectorSize(2048))
	if err != nil {
		return err
	}
	fs, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition: 0, FSType: filesystem.TypeISO9660})
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, name := range names {
		if dir := path.Dir(name); dir != "/" && dir != "." {
			if err := fs.Mkdir(dir); err != nil {
				return err
			}
		}
		fh, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(files[name])
		if err == nil {
			_, err = fh.Write(b)
		}
		fh.Close()
		if err != nil {
			return err
		}
	}
	isofs, ok := fs.(*iso9660fs.FileSystem)
	if !ok {
		return fmt.Errorf("unexpected filesystem type %T", fs)
	}
	return isofs.Finalize(iso9660fs.FinalizeOptions{
		RockRidge: true, Joliet: true, VolumeIdentifier: "cidata"})
}

// buildSeed writes user-data + meta-data (+ vendor-data when present)
// into the VM dir and packs them as seed.iso, returning its path.
func buildSeed(vmDir, userData, metaData, vendorData string) (string, error) {
	ud := filepath.Join(vmDir, "user-data")
	md := filepath.Join(vmDir, "meta-data")
	if err := os.WriteFile(ud, []byte(userData), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(md, []byte(metaData), 0o644); err != nil {
		return "", err
	}
	files := map[string]string{"/user-data": ud, "/meta-data": md}
	if vendorData != "" {
		vd := filepath.Join(vmDir, "vendor-data")
		if err := os.WriteFile(vd, []byte(vendorData), 0o644); err != nil {
			return "", err
		}
		files["/vendor-data"] = vd
	}
	seed := filepath.Join(vmDir, "seed.iso")
	if err := writeSeedISO(seed, files); err != nil {
		return "", err
	}
	return seed, nil
}
