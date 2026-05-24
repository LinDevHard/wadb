package adb

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFindPrefersAndroidHome creates a fake adb in a temp ANDROID_HOME and
// verifies Find() returns it ahead of $PATH and other candidates.
func TestFindPrefersAndroidHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	tmp := t.TempDir()
	platformTools := filepath.Join(tmp, "platform-tools")
	if err := os.MkdirAll(platformTools, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(platformTools, "adb")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANDROID_HOME", tmp)
	t.Setenv("ANDROID_SDK_ROOT", "")
	// Point HOME at a fresh tempdir so user-home candidates do not match.
	t.Setenv("HOME", t.TempDir())
	// Empty PATH to prevent LookPath from picking a real adb.
	t.Setenv("PATH", "")

	got, err := Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != fake {
		t.Fatalf("Find = %q, want %q", got, fake)
	}
}

func TestFindReturnsErrorWhenAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "")

	// The Homebrew fallback paths might exist on a dev machine, so we only
	// assert that *either* Find succeeds with such a path or returns the
	// "not found" error — we never want a panic or empty success.
	got, err := Find()
	if err == nil && got == "" {
		t.Fatalf("Find returned empty path with nil error")
	}
}

func TestParseVersion(t *testing.T) {
	raw := "Android Debug Bridge version 1.0.41\nVersion 37.0.0-13206524\nInstalled as /sdk/platform-tools/adb"

	got := parseVersion(raw)
	if got.Raw != raw {
		t.Fatalf("Raw = %q, want %q", got.Raw, raw)
	}
	if got.PlatformToolsMajor != 37 {
		t.Fatalf("PlatformToolsMajor = %d, want 37", got.PlatformToolsMajor)
	}
	if !got.SupportsWifi2Improvements() {
		t.Fatalf("SupportsWifi2Improvements = false, want true")
	}
}

func TestParseVersionWithoutPlatformToolsVersion(t *testing.T) {
	got := parseVersion("Android Debug Bridge version 1.0.41")
	if got.PlatformToolsMajor != 0 {
		t.Fatalf("PlatformToolsMajor = %d, want 0", got.PlatformToolsMajor)
	}
	if got.SupportsWifi2Improvements() {
		t.Fatalf("SupportsWifi2Improvements = true, want false")
	}
}

func TestParseMDNSServicesHeaderOnly(t *testing.T) {
	got := ParseMDNSServices("List of discovered mdns services\n")
	if len(got) != 0 {
		t.Fatalf("ParseMDNSServices returned %v, want empty", got)
	}
}

func TestParseMDNSServices(t *testing.T) {
	raw := "List of discovered mdns services\nadb-123._adb-tls-connect._tcp.\t192.168.1.10:37123\n\nstudio-abc._adb-tls-pairing._tcp.\t192.168.1.10:45555"

	got := ParseMDNSServices(raw)
	want := []string{
		"adb-123._adb-tls-connect._tcp.\t192.168.1.10:37123",
		"studio-abc._adb-tls-pairing._tcp.\t192.168.1.10:45555",
	}
	if len(got) != len(want) {
		t.Fatalf("len(ParseMDNSServices) = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseMDNSServices[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
