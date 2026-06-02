package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lindevhard/wadb/internal/adb"
	"github.com/lindevhard/wadb/internal/mdns"
)

func TestRunPairOnlyUsesExplicitADBPathAndSkipsConnect(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	wantADB := "/tmp/custom-adb"
	var started, paired, connected bool

	getADBVersion = func(_ context.Context, adbPath string) (adb.VersionInfo, error) {
		if adbPath != wantADB {
			t.Fatalf("Version adbPath = %q, want %q", adbPath, wantADB)
		}
		return adb.VersionInfo{Raw: "Version 37.0.0", PlatformToolsMajor: 37}, nil
	}
	adbStartServer = func(_ context.Context, adbPath string) error {
		if adbPath != wantADB {
			t.Fatalf("StartServer adbPath = %q, want %q", adbPath, wantADB)
		}
		started = true
		return nil
	}
	browsePairing = func(_ context.Context, _ string, _ mdns.Logf) (mdns.Endpoint, error) {
		return mdns.Endpoint{Host: "192.168.1.10", Port: 37123}, nil
	}
	adbPair = func(_ context.Context, adbPath, host string, port int, password string) error {
		if adbPath != wantADB {
			t.Fatalf("Pair adbPath = %q, want %q", adbPath, wantADB)
		}
		if host != "192.168.1.10" || port != 37123 {
			t.Fatalf("Pair endpoint = %s:%d, want 192.168.1.10:37123", host, port)
		}
		if password == "" {
			t.Fatal("Pair password is empty")
		}
		paired = true
		return nil
	}
	adbConnect = func(context.Context, string, string, int) (string, error) {
		connected = true
		return "", nil
	}

	withDiscardedOutput(t, func() {
		err := run(runOptions{
			ADBPath:        wantADB,
			PairingTimeout: time.Second,
			ConnectTimeout: time.Second,
			PairOnly:       true,
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if !started {
		t.Fatal("StartServer was not called")
	}
	if !paired {
		t.Fatal("Pair was not called")
	}
	if connected {
		t.Fatal("Connect was called in pair-only mode")
	}
}

func TestDoctorUsesExplicitADBPath(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	wantADB := "/tmp/doctor-adb"
	var started, listed bool

	getADBVersion = func(_ context.Context, adbPath string) (adb.VersionInfo, error) {
		if adbPath != wantADB {
			t.Fatalf("Version adbPath = %q, want %q", adbPath, wantADB)
		}
		return adb.VersionInfo{Raw: "Version 37.0.0", PlatformToolsMajor: 37}, nil
	}
	adbStartServer = func(_ context.Context, adbPath string) error {
		if adbPath != wantADB {
			t.Fatalf("StartServer adbPath = %q, want %q", adbPath, wantADB)
		}
		started = true
		return nil
	}
	adbMDNSServices = func(_ context.Context, adbPath string) (string, error) {
		if adbPath != wantADB {
			t.Fatalf("MDNSServices adbPath = %q, want %q", adbPath, wantADB)
		}
		listed = true
		return "List of discovered mdns services\n", nil
	}

	withDiscardedOutput(t, func() {
		if err := doctor(wantADB, false); err != nil {
			t.Fatalf("doctor: %v", err)
		}
	})

	if !started {
		t.Fatal("StartServer was not called")
	}
	if !listed {
		t.Fatal("MDNSServices was not called")
	}
}

func TestLoadEnvOptions(t *testing.T) {
	t.Setenv("WADB_ADB", " /tmp/env-adb ")
	t.Setenv("WADB_PAIR_ONLY", "true")
	t.Setenv("WADB_QR_ASCII", "1")
	t.Setenv("WADB_VERBOSE", "false")
	t.Setenv("WADB_PAIR_TIMEOUT", "3m")
	t.Setenv("WADB_CONNECT_TIMEOUT", "45s")

	got, err := loadEnvOptions()
	if err != nil {
		t.Fatalf("loadEnvOptions: %v", err)
	}
	if got.ADBPath != "/tmp/env-adb" {
		t.Fatalf("ADBPath = %q, want /tmp/env-adb", got.ADBPath)
	}
	if !got.PairOnly {
		t.Fatal("PairOnly = false, want true")
	}
	if !got.QRASCII {
		t.Fatal("QRASCII = false, want true")
	}
	if got.Verbose {
		t.Fatal("Verbose = true, want false")
	}
	if got.PairingTimeout != 3*time.Minute {
		t.Fatalf("PairingTimeout = %s, want 3m", got.PairingTimeout)
	}
	if got.ConnectTimeout != 45*time.Second {
		t.Fatalf("ConnectTimeout = %s, want 45s", got.ConnectTimeout)
	}
}

func TestLoadEnvOptionsRejectsInvalidValues(t *testing.T) {
	t.Setenv("WADB_PAIR_ONLY", "sometimes")
	if _, err := loadEnvOptions(); err == nil {
		t.Fatal("loadEnvOptions accepted invalid boolean")
	}

	t.Setenv("WADB_PAIR_ONLY", "")
	t.Setenv("WADB_CONNECT_TIMEOUT", "soon")
	if _, err := loadEnvOptions(); err == nil {
		t.Fatal("loadEnvOptions accepted invalid duration")
	}
}

func TestCLIFlagsOverrideEnvDefaults(t *testing.T) {
	envOpts := runOptions{
		ADBPath:        "/tmp/env-adb",
		PairingTimeout: 3 * time.Minute,
		ConnectTimeout: 45 * time.Second,
		PairOnly:       true,
		QRASCII:        true,
		Verbose:        true,
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	adbPath := fs.String("adb", envOpts.ADBPath, "")
	pairOnly := fs.Bool("pair-only", envOpts.PairOnly, "")
	qrASCII := fs.Bool("qr-ascii", envOpts.QRASCII, "")
	verbose := fs.Bool("verbose", envOpts.Verbose, "")
	pairingTimeout := fs.Duration("pair-timeout", envOpts.PairingTimeout, "")
	connectTimeout := fs.Duration("connect-timeout", envOpts.ConnectTimeout, "")

	if err := fs.Parse([]string{
		"--adb", "/tmp/flag-adb",
		"--pair-only=false",
		"--qr-ascii=false",
		"--verbose=false",
		"--pair-timeout", "10s",
		"--connect-timeout", "20s",
	}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := runOptions{
		ADBPath:        *adbPath,
		PairingTimeout: *pairingTimeout,
		ConnectTimeout: *connectTimeout,
		PairOnly:       *pairOnly,
		QRASCII:        *qrASCII,
		Verbose:        *verbose,
	}
	if got.ADBPath != "/tmp/flag-adb" {
		t.Fatalf("ADBPath = %q, want /tmp/flag-adb", got.ADBPath)
	}
	if got.PairOnly || got.QRASCII || got.Verbose {
		t.Fatalf("bool flags did not override env defaults: %+v", got)
	}
	if got.PairingTimeout != 10*time.Second {
		t.Fatalf("PairingTimeout = %s, want 10s", got.PairingTimeout)
	}
	if got.ConnectTimeout != 20*time.Second {
		t.Fatalf("ConnectTimeout = %s, want 20s", got.ConnectTimeout)
	}
}

func TestRunPrefersConnectEndpointFromPairingHost(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	var tried []string

	browsePairing = func(context.Context, string, mdns.Logf) (mdns.Endpoint, error) {
		return mdns.Endpoint{Host: "192.168.1.20", Port: 37123}, nil
	}
	browseConnect = func(context.Context, time.Duration, mdns.Logf) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{
			{Host: "192.168.1.99", Port: 40001},
			{Host: "192.168.1.20", Port: 40002},
		}, nil
	}
	adbConnect = func(_ context.Context, _ string, host string, port int) (string, error) {
		tried = append(tried, host)
		return "connected", nil
	}
	adbDeviceName = func(context.Context, string, string) (string, error) {
		return "Google Pixel 8", nil
	}

	withDiscardedOutput(t, func() {
		err := run(runOptions{
			ADBPath:        "/tmp/custom-adb",
			PairingTimeout: time.Second,
			ConnectTimeout: time.Second,
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if len(tried) != 1 {
		t.Fatalf("tried %d endpoints, want 1: %v", len(tried), tried)
	}
	if tried[0] != "192.168.1.20" {
		t.Fatalf("first connect host = %q, want pairing host", tried[0])
	}
}

func TestRunFallsBackWhenPreferredConnectEndpointFails(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	var tried []string

	browsePairing = func(context.Context, string, mdns.Logf) (mdns.Endpoint, error) {
		return mdns.Endpoint{Host: "192.168.1.20", Port: 37123}, nil
	}
	browseConnect = func(context.Context, time.Duration, mdns.Logf) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{
			{Host: "192.168.1.99", Port: 40001},
			{Host: "192.168.1.20", Port: 40002},
			{Host: "192.168.1.21", Port: 40003},
		}, nil
	}
	adbConnect = func(_ context.Context, _ string, host string, port int) (string, error) {
		addr := fmt.Sprintf("%s:%d", host, port)
		tried = append(tried, addr)
		if addr == "192.168.1.20:40002" {
			return "", errors.New("preferred endpoint refused connection")
		}
		return "connected", nil
	}

	withDiscardedOutput(t, func() {
		err := run(runOptions{
			ADBPath:        "/tmp/custom-adb",
			PairingTimeout: time.Second,
			ConnectTimeout: time.Second,
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	want := []string{"192.168.1.20:40002", "192.168.1.99:40001"}
	if len(tried) != len(want) {
		t.Fatalf("tried endpoints = %v, want %v", tried, want)
	}
	for i := range want {
		if tried[i] != want[i] {
			t.Fatalf("tried[%d] = %q, want %q; all tried %v", i, tried[i], want[i], tried)
		}
	}
}

func TestRunReturnsAllConnectFailures(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	browsePairing = func(context.Context, string, mdns.Logf) (mdns.Endpoint, error) {
		return mdns.Endpoint{Host: "192.168.1.20", Port: 37123}, nil
	}
	browseConnect = func(context.Context, time.Duration, mdns.Logf) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{
			{Host: "192.168.1.20", Port: 40002},
			{Host: "192.168.1.99", Port: 40001},
		}, nil
	}
	adbConnect = func(_ context.Context, _ string, host string, port int) (string, error) {
		return "", fmt.Errorf("connect %s:%d failed", host, port)
	}

	var err error
	withDiscardedOutput(t, func() {
		err = run(runOptions{
			ADBPath:        "/tmp/custom-adb",
			PairingTimeout: time.Second,
			ConnectTimeout: time.Second,
		})
	})
	if err == nil {
		t.Fatal("run succeeded, want connect failure")
	}

	got := err.Error()
	for _, want := range []string{
		"failed to connect to 2 discovered endpoint(s)",
		"connect 192.168.1.20:40002 failed",
		"connect 192.168.1.99:40001 failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error %q does not contain %q", got, want)
		}
	}
}

func TestRunIgnoresDeviceNameFailureAfterSuccessfulConnect(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	var nameSerial string

	browsePairing = func(context.Context, string, mdns.Logf) (mdns.Endpoint, error) {
		return mdns.Endpoint{Host: "192.168.1.20", Port: 37123}, nil
	}
	browseConnect = func(context.Context, time.Duration, mdns.Logf) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{{Host: "192.168.1.20", Port: 40002}}, nil
	}
	adbConnect = func(context.Context, string, string, int) (string, error) {
		return "connected", nil
	}
	adbDeviceName = func(_ context.Context, _ string, serial string) (string, error) {
		nameSerial = serial
		return "", errors.New("getprop unavailable")
	}

	withDiscardedOutput(t, func() {
		err := run(runOptions{
			ADBPath:        "/tmp/custom-adb",
			PairingTimeout: time.Second,
			ConnectTimeout: time.Second,
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if nameSerial != "192.168.1.20:40002" {
		t.Fatalf("DeviceName serial = %q, want 192.168.1.20:40002", nameSerial)
	}
}

func replaceHooks(t *testing.T) func() {
	t.Helper()
	oldFindADB := findADB
	oldGetADBVersion := getADBVersion
	oldADBStartServer := adbStartServer
	oldADBMDNSServices := adbMDNSServices
	oldADBPair := adbPair
	oldADBConnect := adbConnect
	oldADBDeviceName := adbDeviceName
	oldBrowsePairing := browsePairing
	oldBrowseConnect := browseConnect

	findADB = func() (string, error) {
		t.Fatal("findADB should not be called when ADBPath is explicit")
		return "", nil
	}
	getADBVersion = func(context.Context, string) (adb.VersionInfo, error) {
		return adb.VersionInfo{}, nil
	}
	adbStartServer = func(context.Context, string) error { return nil }
	adbMDNSServices = func(context.Context, string) (string, error) { return "", nil }
	adbPair = func(context.Context, string, string, int, string) error { return nil }
	adbConnect = func(context.Context, string, string, int) (string, error) { return "", nil }
	adbDeviceName = func(context.Context, string, string) (string, error) { return "", nil }
	browsePairing = func(context.Context, string, mdns.Logf) (mdns.Endpoint, error) {
		return mdns.Endpoint{}, nil
	}
	browseConnect = func(context.Context, time.Duration, mdns.Logf) ([]mdns.Endpoint, error) {
		return nil, nil
	}

	return func() {
		findADB = oldFindADB
		getADBVersion = oldGetADBVersion
		adbStartServer = oldADBStartServer
		adbMDNSServices = oldADBMDNSServices
		adbPair = oldADBPair
		adbConnect = oldADBConnect
		adbDeviceName = oldADBDeviceName
		browsePairing = oldBrowsePairing
		browseConnect = oldBrowseConnect
	}
}

func withDiscardedOutput(t *testing.T, fn func()) {
	t.Helper()
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	os.Stdout = devNull
	os.Stderr = devNull
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	fn()
}
