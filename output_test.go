package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lindevhard/wadb/internal/adb"
	"github.com/lindevhard/wadb/internal/mdns"
)

func TestExecuteCLIWritesJSONForInvalidArguments(t *testing.T) {
	t.Setenv("WADB_JSON", "")
	stdout := captureStdout(t, func() {
		if code := executeCLI([]string{"connect", "--output", "json", "--does-not-exist"}); code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	response := decodeJSONResponse(t, stdout)
	if response.SchemaVersion != jsonSchemaVersion || response.OK {
		t.Fatalf("response = %+v", response)
	}
	if response.Action != "connect" || response.Error == nil || response.Error.Code != "invalid_arguments" {
		t.Fatalf("error response = %+v", response)
	}
}

func TestExecuteCLIHelpIsSuccessful(t *testing.T) {
	withDiscardedOutput(t, func() {
		if code := executeCLI([]string{"--help"}); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	})

	stdout := captureStdout(t, func() {
		if code := executeCLI([]string{"--help", "--output", "json"}); code != 0 {
			t.Fatalf("JSON help exit code = %d, want 0", code)
		}
	})
	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "help" {
		t.Fatalf("response = %+v", response)
	}
}

func TestExecuteCLIRejectsInteractiveQRPairingInJSONMode(t *testing.T) {
	stdout := captureStdout(t, func() {
		if code := executeCLI([]string{"--output", "json"}); code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	response := decodeJSONResponse(t, stdout)
	if response.Action != "pair" || response.Error == nil || response.Error.Code != "interactive_required" {
		t.Fatalf("response = %+v", response)
	}
}

func TestExecuteCLIRejectsQRPairingInNonInteractiveMode(t *testing.T) {
	stdout := captureStdout(t, func() {
		if code := executeCLI([]string{"--non-interactive", "--output", "json"}); code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	response := decodeJSONResponse(t, stdout)
	if response.Error == nil || response.Error.Code != "interactive_required" || !response.Error.RequiresUserAction {
		t.Fatalf("response = %+v", response)
	}
}

func TestCapabilitiesJSONDescribesAgentContract(t *testing.T) {
	stdout := captureStdout(t, func() {
		if code := executeCLI([]string{"capabilities", "--non-interactive", "--output", "json"}); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	})

	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "capabilities" {
		t.Fatalf("response = %+v", response)
	}
	result := response.Result.(map[string]any)
	if result["schema_version"] != float64(jsonSchemaVersion) || result["non_interactive_flag"] != "--non-interactive" {
		t.Fatalf("capabilities = %#v", result)
	}
	commands := result["commands"].([]any)
	if len(commands) < 6 {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestNonInteractivePairRejectsTerminalStdinBeforeADBSetup(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()
	stdinIsTerminal = func() bool { return true }
	adbStartServer = func(context.Context, string) error {
		t.Fatal("adb server started before non-interactive input was validated")
		return nil
	}

	err := pairByCode(runOptions{ADBPath: "/tmp/adb", NonInteractive: true, Output: "json"}, "192.168.1.20:37123")
	var commandErr commandError
	if !errors.As(err, &commandErr) || commandErr.code != "interactive_required" {
		t.Fatalf("error = %#v", err)
	}
}

func TestNonInteractivePairReadsRedirectedStdin(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()
	stdinIsTerminal = func() bool { return false }
	browseConnect = func(context.Context, time.Duration, mdns.Options) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{{Instance: "adb-RF8M1234ABC-x", Host: "192.168.1.20", Port: 40002}}, nil
	}

	input, err := os.CreateTemp(t.TempDir(), "pairing-code-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := input.WriteString("123456\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = input
	defer func() {
		os.Stdin = oldStdin
		_ = input.Close()
	}()

	stdout := captureStdout(t, func() {
		if err := pairByCode(runOptions{ADBPath: "/tmp/adb", NonInteractive: true, Output: "json", ConnectTimeout: time.Second}, "192.168.1.20:37123"); err != nil {
			t.Fatalf("pairByCode: %v", err)
		}
	})
	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "pair" {
		t.Fatalf("response = %+v", response)
	}
}

func TestPublishedOutputSchemaIsValidJSON(t *testing.T) {
	raw, err := os.ReadFile("schemas/wadb-output-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected schema declaration: %#v", schema["$schema"])
	}
}

func TestSchemaCommandUsesEmbeddedSchema(t *testing.T) {
	raw := captureStdout(t, func() {
		if code := executeCLI([]string{"schema"}); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	})
	if raw != string(outputSchemaV1) {
		t.Fatal("schema command output differs from the published schema")
	}

	wrapped := captureStdout(t, func() {
		if code := executeCLI([]string{"schema", "--output", "json"}); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	})
	response := decodeJSONResponse(t, wrapped)
	if !response.OK || response.Action != "schema" {
		t.Fatalf("response = %+v", response)
	}
}

func TestDevicesJSONUsesVersionedEnvelope(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	adbDevices = func(context.Context, string) ([]adb.DeviceEntry, error) {
		return []adb.DeviceEntry{{Serial: "RF8M1234ABC", State: "device", Model: "Pixel_8"}}, nil
	}

	stdout := captureStdout(t, func() {
		if err := devices(runOptions{ADBPath: "/tmp/adb", Output: "json", ScanTimeout: time.Millisecond}); err != nil {
			t.Fatalf("devices: %v", err)
		}
	})

	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "devices" || response.Result == nil {
		t.Fatalf("response = %+v", response)
	}
	result := response.Result.(map[string]any)
	devices := result["devices"].([]any)
	if len(devices) != 1 || devices[0].(map[string]any)["id"] != "RF8M1234ABC" {
		t.Fatalf("devices result = %#v", result)
	}
}

func TestLegacyJSONShapeRemainsAnArray(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	adbDevices = func(context.Context, string) ([]adb.DeviceEntry, error) {
		return []adb.DeviceEntry{{Serial: "RF8M1234ABC", State: "device", Model: "Pixel_8"}}, nil
	}
	stdout := captureStdout(t, func() {
		if err := devices(runOptions{ADBPath: "/tmp/adb", JSON: true, ScanTimeout: time.Millisecond}); err != nil {
			t.Fatalf("devices: %v", err)
		}
	})
	var devices []managedDevice
	if err := json.Unmarshal([]byte(stdout), &devices); err != nil {
		t.Fatalf("legacy JSON is not an array: %q: %v", stdout, err)
	}
	if len(devices) != 1 || devices[0].ID != "RF8M1234ABC" {
		t.Fatalf("legacy devices = %+v", devices)
	}
}

func TestConnectJSONRefusesAmbiguousDeviceSelection(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	browseConnect = func(context.Context, time.Duration, mdns.Options) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{
			{Instance: "adb-one-abc", Host: "192.168.1.20", Port: 40001},
			{Instance: "adb-two-def", Host: "192.168.1.21", Port: 40002},
		}, nil
	}
	adbConnect = func(context.Context, string, string, int) (string, error) {
		t.Fatal("adb connect ran before an ambiguous JSON selection was resolved")
		return "", nil
	}

	err := connect(runOptions{ADBPath: "/tmp/adb", Output: "json", ConnectTimeout: time.Second})
	if err == nil {
		t.Fatal("connect succeeded with multiple devices and no selector")
	}
	var commandErr commandError
	if !errors.As(err, &commandErr) || commandErr.code != "multiple_devices" {
		t.Fatalf("error = %#v, want multiple_devices", err)
	}
}

func TestPairByCodeJSONReportsPairAndConnection(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	readPairingCode = func() (string, error) { return "123456", nil }
	browseConnect = func(context.Context, time.Duration, mdns.Options) ([]mdns.Endpoint, error) {
		return []mdns.Endpoint{{Instance: "adb-RF8M1234ABC-x", Host: "192.168.1.20", Port: 40002}}, nil
	}
	adbConnect = func(context.Context, string, string, int) (string, error) {
		return "connected", nil
	}
	adbDeviceName = func(context.Context, string, string) (string, error) {
		return "Pixel 8", nil
	}

	stdout := captureStdout(t, func() {
		err := pairByCode(runOptions{ADBPath: "/tmp/adb", Output: "json", ConnectTimeout: time.Second}, "192.168.1.20:37123")
		if err != nil {
			t.Fatalf("pairByCode: %v", err)
		}
	})

	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "pair" {
		t.Fatalf("response = %+v", response)
	}
	result := response.Result.(map[string]any)
	if result["paired"] != true || result["connected"] != true {
		t.Fatalf("pair result = %#v", result)
	}
}

func TestDoctorJSONUsesVersionedEnvelope(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	getADBVersion = func(context.Context, string) (adb.VersionInfo, error) {
		return adb.VersionInfo{PlatformToolsMajor: 37}, nil
	}

	stdout := captureStdout(t, func() {
		if err := doctor(runOptions{ADBPath: "/tmp/adb", Output: "json"}); err != nil {
			t.Fatalf("doctor: %v", err)
		}
	})
	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "doctor" {
		t.Fatalf("response = %+v", response)
	}
	result := response.Result.(map[string]any)
	if result["adb_path"] != "/tmp/adb" || result["adb_server"] != "running" {
		t.Fatalf("doctor result = %#v", result)
	}
}

func TestConnectAndDisconnectJSONUseVersionedEnvelope(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()

	adbConnect = func(context.Context, string, string, int) (string, error) {
		return "connected", nil
	}
	connectOutput := captureStdout(t, func() {
		err := connect(runOptions{ADBPath: "/tmp/adb", Device: "192.168.1.20:40002", Output: "json"})
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
	})
	connectResponse := decodeJSONResponse(t, connectOutput)
	if !connectResponse.OK || connectResponse.Action != "connect" {
		t.Fatalf("connect response = %+v", connectResponse)
	}

	adbDisconnect = func(context.Context, string, string) (string, error) {
		return "disconnected", nil
	}
	disconnectOutput := captureStdout(t, func() {
		err := disconnect(runOptions{ADBPath: "/tmp/adb", Device: "192.168.1.20:40002", Output: "json"})
		if err != nil {
			t.Fatalf("disconnect: %v", err)
		}
	})
	disconnectResponse := decodeJSONResponse(t, disconnectOutput)
	if !disconnectResponse.OK || disconnectResponse.Action != "disconnect" {
		t.Fatalf("disconnect response = %+v", disconnectResponse)
	}
}

func TestVersionJSONUsesVersionedEnvelope(t *testing.T) {
	stdout := captureStdout(t, func() {
		if code := executeCLI([]string{"--version", "--output", "json"}); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	})
	response := decodeJSONResponse(t, stdout)
	if !response.OK || response.Action != "version" {
		t.Fatalf("response = %+v", response)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	output, err := os.CreateTemp(t.TempDir(), "stdout-*.json")
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = output
	defer func() { os.Stdout = oldStdout }()

	fn()
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func decodeJSONResponse(t *testing.T, raw string) jsonEnvelope {
	t.Helper()
	var response jsonEnvelope
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("decode JSON %q: %v", raw, err)
	}
	return response
}
