package mdns

import (
	"strings"
	"testing"
)

func TestCheckInterfaceRejectsUnknownName(t *testing.T) {
	err := CheckInterface("wadb-does-not-exist0")
	if err == nil {
		t.Fatal("CheckInterface accepted an unknown interface")
	}
	if !strings.Contains(err.Error(), "wadb-does-not-exist0") {
		t.Fatalf("error %q does not name the interface", err)
	}
}

func TestCheckInterfaceAcceptsMulticastInterface(t *testing.T) {
	ifaces, err := MulticastInterfaces()
	if err != nil {
		t.Fatalf("MulticastInterfaces: %v", err)
	}
	if len(ifaces) == 0 {
		t.Skip("no multicast-capable interface on this host")
	}

	for _, iface := range ifaces {
		if iface.Name == "" {
			t.Fatalf("interface has no name: %+v", iface)
		}
		if len(iface.IPs) == 0 {
			t.Fatalf("interface %q has no addresses", iface.Name)
		}
		if err := CheckInterface(iface.Name); err != nil {
			t.Fatalf("CheckInterface(%q): %v", iface.Name, err)
		}
	}
}

func TestResolverRejectsUnknownInterfaceBeforeBrowsing(t *testing.T) {
	if _, err := (Options{Iface: "wadb-does-not-exist0"}).resolver(); err == nil {
		t.Fatal("resolver accepted an unknown interface")
	}
}

func TestPreferHostMovesPreferredEndpointsFirst(t *testing.T) {
	endpoints := []Endpoint{
		{Host: "192.168.1.99", Port: 40001},
		{Host: "192.168.1.20", Port: 40002},
		{Host: "192.168.1.99", Port: 40003},
		{Host: "192.168.1.20", Port: 40004},
	}

	got := PreferHost(endpoints, "192.168.1.20")
	want := []Endpoint{
		{Host: "192.168.1.20", Port: 40002},
		{Host: "192.168.1.20", Port: 40004},
		{Host: "192.168.1.99", Port: 40001},
		{Host: "192.168.1.99", Port: 40003},
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PreferHost[%d] = %+v, want %+v; got all %+v", i, got[i], want[i], got)
		}
	}
}

func TestPreferHostKeepsOrderWithoutPreferredHost(t *testing.T) {
	endpoints := []Endpoint{
		{Host: "192.168.1.99", Port: 40001},
		{Host: "192.168.1.98", Port: 40002},
	}

	got := PreferHost(endpoints, "192.168.1.20")
	for i := range endpoints {
		if got[i] != endpoints[i] {
			t.Fatalf("PreferHost changed order: got %+v, want %+v", got, endpoints)
		}
	}
}
