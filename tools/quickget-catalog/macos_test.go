// macos_test.go — macosRecoveryLink with the appleSession/appleInfo
// seams stubbed (no network): dmg link + AssetToken extraction.
package main

import (
	"errors"
	"testing"
)

func fakeApple(t *testing.T, fields []string, sessErr, infoErr error) {
	t.Helper()
	oldS, oldI := appleSession, appleInfo
	appleSession = func() (string, error) { return "sess", sessErr }
	appleInfo = func(_, _, _, _ string) ([]string, error) { return fields, infoErr }
	t.Cleanup(func() { appleSession, appleInfo = oldS, oldI })
}

func TestMacosRecoveryLink(t *testing.T) {
	fakeApple(t, []string{
		"https://oscdn.apple.com/x/RecoveryImage.dmg",
		"expires-token-dmg",
		"https://oscdn.apple.com/x/RecoveryImage.chunklist",
		"expires-token-cl",
	}, nil, nil)
	link, sess := macosRecoveryLink("sonoma")
	if link != "https://oscdn.apple.com/x/RecoveryImage.dmg" || sess != "expires-token-dmg" {
		t.Fatalf("link=%q sess=%q", link, sess)
	}
}

func TestMacosRecoveryLinkDegrade(t *testing.T) {
	// Unknown release → no board → empty (get_macos case-table *).
	if link, sess := macosRecoveryLink("nope"); link != "" || sess != "" {
		t.Fatalf("unknown release link=%q sess=%q", link, sess)
	}
	// Session failure degrades to empty (curl capture parity — the
	// caller's web_check "" fails into FAIL).
	fakeApple(t, nil, errors.New("no session"), nil)
	if link, _ := macosRecoveryLink("sonoma"); link != "" {
		t.Fatalf("session failure link=%q", link)
	}
	// Info failure also degrades.
	fakeApple(t, nil, nil, errors.New("post failed"))
	if link, _ := macosRecoveryLink("sonoma"); link != "" {
		t.Fatalf("info failure link=%q", link)
	}
	// Response without a dmg token → empty link.
	fakeApple(t, []string{"https://oscdn.apple.com/x/RecoveryImage.chunklist"}, nil, nil)
	if link, _ := macosRecoveryLink("sonoma"); link != "" {
		t.Fatalf("no-dmg link=%q", link)
	}
}
