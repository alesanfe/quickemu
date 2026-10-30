// macos.go — the get_macos show/test half for catalog url/check:
// resolve the dmg link through Apple's osrecovery session handshake.
// The download pipeline (dmg→img, OpenCore/OVMF, conf) lives in
// tools/quickget-vm/macos.go; the board table + handshake are shared
// in qelib (get_macos case table contract).
package main

import (
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// appleSession/appleInfo: seams for tests — production delegates to
// the shared qelib handshake on ProbeClient.
var appleSession = func() (string, error) {
	return qelib.AppleSession(qelib.ProbeClient)
}
var appleInfo = func(session, mlb, boardID, osType string) ([]string, error) {
	return qelib.AppleInfo(qelib.ProbeClient, session, mlb, boardID, osType)
}

// macosRecoveryLink resolves the dmg download link and its AssetToken
// session for a macOS release (get_macos's downloadLink/downloadSession).
// Errors degrade to "" like the Bash curl captures — the caller's
// web_check on the empty URL fails into the normal FAIL path.
func macosRecoveryLink(release string) (link, session string) {
	row, ok := qelib.MacBoards[release]
	if !ok {
		return "", ""
	}
	sess, err := appleSession()
	if err != nil {
		return "", ""
	}
	fields, err := appleInfo(sess, row[1], row[0], row[2])
	if err != nil {
		return "", ""
	}
	for _, f := range fields {
		switch {
		case strings.Contains(f, "oscdn") && strings.Contains(f, "dmg"):
			link = f
		case strings.Contains(f, "expires") && strings.Contains(f, "dmg"):
			session = f
		}
	}
	return link, session
}
