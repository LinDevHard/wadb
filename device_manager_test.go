package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lindevhard/wadb/internal/adb"
	"github.com/lindevhard/wadb/internal/mdns"
)

func TestSelectEndpointsByStableID(t *testing.T) {
	endpoints := []mdns.Endpoint{
		{Instance: "adb-RF8M1234ABC-xYz123", Host: "192.168.1.20", Port: 40002},
		{Instance: "adb-tablet-aBc987", Host: "192.168.1.30", Port: 40003},
	}
	got := selectEndpoints(endpoints, "RF8M1234ABC")
	if len(got) != 1 || got[0].Host != "192.168.1.20" {
		t.Fatalf("selectEndpoints by stable ID = %+v", got)
	}
}

func TestConnectAllTriesEveryEndpoint(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()
	var tried []string
	adbConnect = func(_ context.Context, _ string, host string, port int) (string, error) {
		tried = append(tried, fmt.Sprintf("%s:%d", host, port))
		return "connected", nil
	}
	endpoints := []mdns.Endpoint{{Host: "192.168.1.20", Port: 40002}, {Host: "192.168.1.30", Port: 40003}}
	withDiscardedOutput(t, func() {
		if err := connectToEndpointsMode(context.Background(), "/tmp/adb", endpoints, true, false); err != nil {
			t.Fatalf("connectToEndpointsMode: %v", err)
		}
	})
	if len(tried) != 2 {
		t.Fatalf("connect --all tried %v, want both endpoints", tried)
	}
}

func TestConnectDirectAddressSkipsDiscovery(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()
	var connected string
	browseConnect = func(context.Context, time.Duration, mdns.Options) ([]mdns.Endpoint, error) {
		t.Fatal("direct address triggered mDNS discovery")
		return nil, nil
	}
	adbConnect = func(_ context.Context, _ string, host string, port int) (string, error) {
		connected = fmt.Sprintf("%s:%d", host, port)
		return "connected", nil
	}
	withDiscardedOutput(t, func() {
		if err := connect(runOptions{ADBPath: "/tmp/adb", Device: "192.168.1.20:40002"}); err != nil {
			t.Fatalf("connect: %v", err)
		}
	})
	if connected != "192.168.1.20:40002" {
		t.Fatalf("connected %q, want direct address", connected)
	}
}

func TestDisconnectDirectAddress(t *testing.T) {
	restore := replaceHooks(t)
	defer restore()
	var disconnected string
	adbDisconnect = func(_ context.Context, _ string, address string) (string, error) {
		disconnected = address
		return "disconnected", nil
	}
	withDiscardedOutput(t, func() {
		err := disconnect(runOptions{ADBPath: "/tmp/adb", Device: "192.168.1.20:40002", ScanTimeout: time.Millisecond})
		if err != nil {
			t.Fatalf("disconnect: %v", err)
		}
	})
	if disconnected != "192.168.1.20:40002" {
		t.Fatalf("disconnected %q, want direct address", disconnected)
	}
}

func TestMergeManagedDevices(t *testing.T) {
	connected := []adb.DeviceEntry{
		{Serial: "R3CN30ABCDE", State: "device", Model: "SM_S918B"},
		{Serial: "192.168.1.20:40002", State: "device", Model: "Pixel_8_Pro"},
	}
	announced := []mdns.Endpoint{
		{Instance: "adb-pixel-abc", Host: "192.168.1.20", Port: 40002},
		{Instance: "adb-tablet-def", Host: "192.168.1.30", Port: 40003},
	}

	got := mergeManagedDevices(connected, announced)
	if len(got) != 3 {
		t.Fatalf("mergeManagedDevices returned %d rows, want 3: %+v", len(got), got)
	}
	if got[0].Name != "SM S918B" {
		t.Fatalf("USB device name = %q, want SM S918B", got[0].Name)
	}
	if got[1].ID != "pixel" || got[1].Instance != "adb-pixel-abc" || got[1].Status != "connected" || got[1].Name != "Pixel 8 Pro" {
		t.Fatalf("merged connected device = %+v", got[1])
	}
	if got[2].Address != "192.168.1.30:40003" || got[2].Status != "discovered" || got[2].Instance != "adb-tablet-def" {
		t.Fatalf("announced device = %+v", got[2])
	}
}

func TestMergeManagedDevicesDeduplicatesUSBAndWifiByStableID(t *testing.T) {
	connected := []adb.DeviceEntry{{Serial: "RF8M1234ABC", State: "device", Model: "Pixel_8"}}
	announced := []mdns.Endpoint{{Instance: "adb-RF8M1234ABC-random", Host: "192.168.1.20", Port: 40002}}
	got := mergeManagedDevices(connected, announced)
	if len(got) != 1 {
		t.Fatalf("mergeManagedDevices returned %d rows, want one physical device: %+v", len(got), got)
	}
	if got[0].ID != "RF8M1234ABC" || got[0].Transport != "usb,wifi" || !strings.Contains(got[0].Address, "192.168.1.20:40002") {
		t.Fatalf("merged USB/Wi-Fi device = %+v", got[0])
	}
}
