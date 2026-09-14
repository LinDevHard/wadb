package adb

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPairSendsCodeThroughStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	fake := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\n" +
		"test \"$#\" -eq 2 || exit 2\n" +
		"test \"$1\" = pair || exit 3\n" +
		"IFS= read -r code\n" +
		"test \"$code\" = 123456 || exit 4\n" +
		"echo Successfully paired to \"$2\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Pair(context.Background(), fake, "192.168.1.20", 37123, "123456"); err != nil {
		t.Fatalf("Pair: %v", err)
	}
}

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

func TestParseMDNSServiceEntries(t *testing.T) {
	raw := strings.Join([]string{
		"List of discovered mdns services",
		"adb-39131FDJH000GV-vWTUFy\t_adb-tls-connect._tcp\t192.168.1.135:37419",
		"studio-AbCdEf1234   _adb-tls-pairing._tcp.   192.168.1.135:41255",
		"adb-ipv6-device\t_adb-tls-connect._tcp\t[fe80::c87:6e8e:bdef:22e6]:44001",
		"",
		"truncated line",
		"adb-bad-port\t_adb-tls-connect._tcp\t192.168.1.135:not-a-port",
		"adb-no-port\t_adb-tls-connect._tcp\t192.168.1.135",
	}, "\n")

	want := []MDNSService{
		{Instance: "adb-39131FDJH000GV-vWTUFy", Service: "_adb-tls-connect._tcp", Host: "192.168.1.135", Port: 37419},
		{Instance: "studio-AbCdEf1234", Service: "_adb-tls-pairing._tcp", Host: "192.168.1.135", Port: 41255},
		{Instance: "adb-ipv6-device", Service: "_adb-tls-connect._tcp", Host: "fe80::c87:6e8e:bdef:22e6", Port: 44001},
	}

	got := ParseMDNSServiceEntries(raw)
	if len(got) != len(want) {
		t.Fatalf("parsed %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseMDNSServiceEntriesWithoutServices(t *testing.T) {
	if got := ParseMDNSServiceEntries("List of discovered mdns services\n\n"); len(got) != 0 {
		t.Fatalf("parsed %d entries from an empty list: %+v", len(got), got)
	}
}

func TestParseDevices(t *testing.T) {
	raw := strings.Join([]string{
		"List of devices attached",
		"R3CN30ABCDE\tdevice product:dm3qxxx model:SM_S918B device:dm3q transport_id:1",
		"192.168.1.20:40002 device product:husky model:Pixel_8_Pro device:husky transport_id:2",
		"192.168.1.21:40003 offline transport_id:3",
		"ZY123 no permissions (missing udev rules?); see [http://developer.android.com/tools/device.html]",
		"",
	}, "\n")

	got := ParseDevices(raw)
	if len(got) != 4 {
		t.Fatalf("ParseDevices returned %d entries, want 4: %+v", len(got), got)
	}
	if got[0].Serial != "R3CN30ABCDE" || got[0].Model != "SM_S918B" || got[0].State != "device" {
		t.Fatalf("ParseDevices USB entry = %+v", got[0])
	}
	if got[1].Serial != "192.168.1.20:40002" || got[1].Product != "husky" || got[1].Device != "husky" {
		t.Fatalf("ParseDevices TCP entry = %+v", got[1])
	}
	if got[2].State != "offline" || got[2].TransportID != "3" {
		t.Fatalf("ParseDevices offline entry = %+v", got[2])
	}
	if got[3].State != "no permissions" {
		t.Fatalf("ParseDevices no-permissions entry = %+v", got[3])
	}
}

func TestDeviceInfoDisplayName(t *testing.T) {
	tests := []struct {
		name string
		info DeviceInfo
		want string
	}{
		{
			name: "manufacturer model and serial",
			info: DeviceInfo{Manufacturer: "Google", Model: "Pixel 8", Serial: "abc123"},
			want: "Google Pixel 8 (abc123)",
		},
		{
			name: "duplicate manufacturer and model",
			info: DeviceInfo{Manufacturer: "Samsung", Model: "Samsung", Serial: "xyz"},
			want: "Samsung (xyz)",
		},
		{
			name: "serial fallback",
			info: DeviceInfo{Serial: "abc123"},
			want: "abc123",
		},
		{
			name: "empty",
			info: DeviceInfo{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.DisplayName(); got != tt.want {
				t.Fatalf("DisplayName = %q, want %q", got, tt.want)
			}
		})
	}
}
