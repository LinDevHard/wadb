package adb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Find locates the adb binary. It checks well-known SDK locations first,
// then $PATH, and finally Homebrew commandline-tools. Returns the first
// path that exists and is executable.
func Find() (string, error) {
	var candidates []string

	if v := os.Getenv("ANDROID_HOME"); v != "" {
		candidates = append(candidates, filepath.Join(v, "platform-tools", adbBin()))
	}
	if v := os.Getenv("ANDROID_SDK_ROOT"); v != "" {
		candidates = append(candidates, filepath.Join(v, "platform-tools", adbBin()))
	}
	if home, err := os.UserHomeDir(); err == nil {
		switch runtime.GOOS {
		case "darwin":
			candidates = append(candidates, filepath.Join(home, "Library", "Android", "sdk", "platform-tools", adbBin()))
		case "windows":
			if v := os.Getenv("LOCALAPPDATA"); v != "" {
				candidates = append(candidates, filepath.Join(v, "Android", "Sdk", "platform-tools", adbBin()))
			}
		}
		candidates = append(candidates, filepath.Join(home, "Android", "Sdk", "platform-tools", adbBin()))
	}

	for _, c := range candidates {
		if isExecutable(c) {
			return c, nil
		}
	}

	if p, err := exec.LookPath("adb"); err == nil {
		return p, nil
	}

	// Homebrew android-commandlinetools fallback.
	for _, c := range []string{
		"/opt/homebrew/share/android-commandlinetools/platform-tools/adb",
		"/usr/local/share/android-commandlinetools/platform-tools/adb",
	} {
		if isExecutable(c) {
			return c, nil
		}
	}

	return "", errors.New("adb not found: install Android platform-tools or set $ANDROID_HOME")
}

func adbBin() string {
	if runtime.GOOS == "windows" {
		return "adb.exe"
	}
	return "adb"
}

func isExecutable(p string) bool {
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// StartServer ensures the adb daemon is running. Without this the first
// `adb pair` call sometimes hangs on daemon startup before the mDNS
// announce window closes.
func StartServer(ctx context.Context, adbPath string) error {
	cmd := exec.CommandContext(ctx, adbPath, "start-server")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb start-server: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

// Pair runs `adb pair host:port password` and returns the stdout/stderr
// on failure. Success is detected by the "Successfully paired" substring,
// which adb prints on both stdout and stderr across versions.
func Pair(ctx context.Context, adbPath, host string, port int, password string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	cmd := exec.CommandContext(ctx, adbPath, "pair", addr, password)
	out, err := cmd.CombinedOutput()
	combined := string(out)
	if err != nil {
		return fmt.Errorf("adb pair %s failed: %w\n%s", addr, err, strings.TrimSpace(combined))
	}
	if !strings.Contains(combined, "Successfully paired") {
		return fmt.Errorf("adb pair %s: unexpected output: %s", addr, strings.TrimSpace(combined))
	}
	return nil
}

// Connect runs `adb connect host:port`. adb prints "connected to ..." on
// success and "failed to connect" / "cannot connect" on failure, but still
// exits 0 in some versions — so we inspect output.
func Connect(ctx context.Context, adbPath, host string, port int) (string, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	cmd := exec.CommandContext(ctx, adbPath, "connect", addr)
	out, err := cmd.CombinedOutput()
	combined := strings.TrimSpace(string(out))
	if err != nil {
		return combined, fmt.Errorf("adb connect %s: %w: %s", addr, err, combined)
	}
	lower := strings.ToLower(combined)
	if strings.Contains(lower, "failed to connect") || strings.Contains(lower, "cannot connect") {
		return combined, fmt.Errorf("adb connect %s: %s", addr, combined)
	}
	return combined, nil
}
