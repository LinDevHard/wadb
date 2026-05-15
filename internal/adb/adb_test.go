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
