// macos.go: get_macos parity — Apple osrecovery session + recovery
// image download, chunklist verify, dmg→raw convert, OpenCore/OVMF
// fetch, then the conf.
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// Board-ID / MLB / OS_TYPE per macOS release (get_macos case table).
var macBoards = qelib.MacBoards

// generate_id: random uppercase-hex string of n chars.
func generateID(n int) string { return qelib.MacGenID(n) }

// appleRecoverySession: replicate the curl -v cookie scrape — GET the
// osrecovery root and read the `session` cookie value (httpProbe seam).
func appleRecoverySession() (string, error) {
	return qelib.AppleSession(httpProbe)
}

// appleRecoveryInfo: POST the InstallationPayload request and split
// the space-separated response into fields (httpProbe seam).
func appleRecoveryInfo(session, mlb, boardID, osType string) ([]string, error) {
	return qelib.AppleInfo(httpProbe, session, mlb, boardID, osType)
}

// macFetch: web_get with the oscdn headers + AssetToken cookie.
func macFetch(url, dest, token string) int {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	req.Header.Set("Host", "oscdn.apple.com")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "InternetRecovery/1.0")
	req.Header.Set("Cookie", "AssetToken="+token)
	resp, err := httpProbe.Do(req)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		fmt.Printf("download failed: HTTP %d\n", resp.StatusCode)
		return 1
	}
	f, err := os.Create(dest)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		fmt.Println(err)
		return 1
	}
	return 0
}

// getMacos: the full get_macos pipeline.
func getMacos(release, dir, arch string) int {
	row, ok := macBoards[release]
	if !ok {
		fmt.Printf("ERROR! Unknown release: %s\n", release)
		return 1
	}
	boardID, mlb, osType := row[0], row[1], row[2]
	if dir == "" {
		dir = "macos-" + release
	}
	os.MkdirAll(dir, 0755)
	session, err := appleRecoverySession()
	if err != nil {
		fmt.Println("ERROR! Could not get Apple recovery session:", err)
		return 1
	}
	fields, err := appleRecoveryInfo(session, mlb, boardID, osType)
	if err != nil {
		fmt.Println("ERROR! RecoveryImage request failed:", err)
		return 1
	}
	var dmgLink, dmgSession, clLink, clSession string
	for _, f := range fields {
		switch {
		case strings.Contains(f, "oscdn") && strings.Contains(f, "dmg"):
			dmgLink = f
		case strings.Contains(f, "expires") && strings.Contains(f, "dmg"):
			dmgSession = f
		case strings.Contains(f, "oscdn") && strings.Contains(f, "chunklist"):
			clLink = f
		case strings.Contains(f, "expires") && strings.Contains(f, "chunklist"):
			clSession = f
		}
	}
	if dmgLink == "" || clLink == "" {
		fmt.Println("ERROR! Apple did not return a recovery image URL.")
		return 1
	}
	dmg := filepath.Join(dir, "RecoveryImage.dmg")
	cl := filepath.Join(dir, "RecoveryImage.chunklist")
	img := filepath.Join(dir, "RecoveryImage.img")
	if _, err := os.Stat(img); err != nil {
		fmt.Printf("Downloading macOS (%s) RecoveryImage\n", release)
		fmt.Println(" - URL: " + dmgLink)
		if macFetch(dmgLink, dmg, dmgSession) != 0 {
			return 1
		}
		if macFetch(clLink, cl, clSession) != 0 {
			return 1
		}
		// Chunklist verification — pure-Go port of chunkcheck.py; the
		// script remains under libexec/ for the Bash backend.
		if err := checkChunklist(dmg, cl); err == nil {
			fmt.Println(" - Verification passed")
		} else {
			fmt.Println(" - WARNING! Verification failed, continuing anyway")
		}
		if qimg, err := exec.LookPath("qemu-img"); err == nil {
			fmt.Println(" - Converting RecoveryImage.dmg")
			if err := execCommand(qimg, "convert", dmg, "-O", "raw", img).Run(); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: qemu-img convert failed: %v\n", err)
			}
		}
		os.Remove(dmg)
		os.Remove(cl)
		if _, err := os.Stat(img); err == nil {
			fmt.Println(" - RecoveryImage.img is ready.")
		}
	}
	fmt.Println("Downloading OpenCore & UEFI firmware")
	// Pin to last known-good commit; OVMF_CODE.fd was removed from
	// master on 26 Jan 2026 (same pin as the Bash get_macos).
	const commit = "da4b23b5e92c5b939568700034367e8b7649fe90"
	base := "https://github.com/kholia/OSX-KVM/raw/" + commit + "/"
	fetch(base+"OpenCore/OpenCore.qcow2", filepath.Join(dir, "OpenCore.qcow2"), "")
	fetch(base+"OVMF_CODE.fd", filepath.Join(dir, "OVMF_CODE.fd"), "")
	if _, err := os.Stat(filepath.Join(dir, "OVMF_VARS-1920x1080.fd")); err != nil {
		fetch(base+"OVMF_VARS-1920x1080.fd", filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), "")
	}
	if noConf {
		return 0
	}
	return mkconf("macos", release, "", "RecoveryImage.img", "", dir, arch, cloudInitFlag)
}

// checkChunklist verifies a dmg against a CNKL chunklist — pure-Go
// equivalent of libexec/quickemu/chunkcheck.py.
const chunklistPubkeyV1Hex = "C3E748CAD9CD384329E10E25A91E43E1A762FF529ADE578C935BDDF9B13F2179D4855E6FC89E9E29CA12517D17DFA1EDCE0BEBF0EA7B461FFE61D94E2BDF72C196F89ACD3536B644064014DAE25A15DB6BB0852ECBD120916318D1CCDEA3C84C92ED743FC176D0BACA920D3FCF3158AFF731F88CE0623182A8ED67E650515F75745909F07D415F55FC15A35654D118C55A462D37A3ACDA08612F3F3F6571761EFCCBCC299AEE99B3A4FD6212CCFFF5EF37A2C334E871191F7E1C31960E010A54E86FA3F62E6D6905E1CD57732410A3EB0C6B4DEFDABE9F59BF1618758C751CD56CEF851D1C0EAA1C558E37AC108DA9089863D20E2E7E4BF475EC66FE6B3EFDCF"

func checkChunklist(dmgPath, clPath string) error {
	cl, err := os.Open(clPath)
	if err != nil {
		return err
	}
	defer cl.Close()
	h := sha256.New()
	hdr := make([]byte, 36)
	if _, err := io.ReadFull(cl, hdr); err != nil {
		return err
	}
	h.Write(hdr)
	if string(hdr[:4]) != "CNKL" || binary.LittleEndian.Uint32(hdr[4:8]) != 36 ||
		hdr[8] != 1 || hdr[9] != 1 || (hdr[10] != 1 && hdr[10] != 2) {
		return fmt.Errorf("invalid chunklist header")
	}
	chunkCount := binary.LittleEndian.Uint64(hdr[12:20])
	chunkOffset := binary.LittleEndian.Uint64(hdr[20:28])
	sigOffset := binary.LittleEndian.Uint64(hdr[28:36])
	if chunkCount == 0 || chunkOffset != 36 || sigOffset != 36+36*chunkCount {
		return fmt.Errorf("invalid chunklist geometry")
	}
	// chunkCount attacker-controlled → make(..., 0, chunkCount) would
	// capacity-allocate 36B×count before any signature check (a 1<<32
	// count is ~150GiB — instant OOM). Only the real file size bounds
	// it: header + records + signature must equal st.Size() exactly.
	sigLen := int64(32)
	if hdr[10] == 1 {
		sigLen = 256
	}
	if st, serr := cl.Stat(); serr != nil || st.Size() != int64(sigOffset)+sigLen {
		return fmt.Errorf("invalid chunklist size for %d chunks", chunkCount)
	}
	type chunk struct {
		size   uint32
		digest [32]byte
	}
	chunks := make([]chunk, 0, chunkCount)
	for i := uint64(0); i < chunkCount; i++ {
		var rec [36]byte
		if _, err := io.ReadFull(cl, rec[:]); err != nil {
			return err
		}
		h.Write(rec[:])
		var c chunk
		c.size = binary.LittleEndian.Uint32(rec[:4])
		copy(c.digest[:], rec[4:36])
		chunks = append(chunks, c)
	}
	digest := h.Sum(nil)
	sigMethod := hdr[10]
	if sigMethod == 1 {
		sig := make([]byte, 256)
		if _, err := io.ReadFull(cl, sig); err != nil {
			return err
		}
		for i, j := 0, len(sig)-1; i < j; i, j = i+1, j-1 {
			sig[i], sig[j] = sig[j], sig[i] // little-endian → big.Int
		}
		pubkey, _ := new(big.Int).SetString(chunklistPubkeyV1Hex, 16)
		got := new(big.Int).Exp(new(big.Int).SetBytes(sig), big.NewInt(0x10001), pubkey)
		// PKCS#1 v1.5: 0x01 FF...FF 00 <SHA-256 DigestInfo prefix> 00*8 <digest>
		prefix := make([]byte, 0, 256)
		prefix = append(prefix, 0x01)
		for i := 0; i < 202; i++ {
			prefix = append(prefix, 0xFF)
		}
		prefix = append(prefix, 0x00, 0x30, 0x31, 0x30, 0x0D, 0x06, 0x09, 0x60,
			0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20)
		for i := 0; i < 32; i++ {
			prefix = append(prefix, 0x00)
		}
		want := new(big.Int).Or(new(big.Int).SetBytes(prefix), new(big.Int).SetBytes(digest))
		if got.Cmp(want) != 0 {
			return fmt.Errorf("chunklist RSA signature mismatch")
		}
	} else {
		sig := make([]byte, 32)
		if _, err := io.ReadFull(cl, sig); err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(sig, digest) != 1 {
			return fmt.Errorf("chunklist digest mismatch")
		}
	}
	if n, _ := cl.Read(make([]byte, 1)); n != 0 {
		return fmt.Errorf("trailing bytes after chunklist signature")
	}

	dmg, err := os.Open(dmgPath)
	if err != nil {
		return err
	}
	defer dmg.Close()
	for i, c := range chunks {
		buf := make([]byte, c.size)
		if _, err := io.ReadFull(dmg, buf); err != nil {
			return fmt.Errorf("chunk %d short: %w", i, err)
		}
		sum := sha256.Sum256(buf)
		if sum != c.digest {
			return fmt.Errorf("chunk %d sha256 mismatch", i)
		}
	}
	if n, _ := dmg.Read(make([]byte, 1)); n != 0 {
		return fmt.Errorf("trailing bytes after last chunk")
	}
	return nil
}
