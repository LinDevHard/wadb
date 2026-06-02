package mdns

import "testing"

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
