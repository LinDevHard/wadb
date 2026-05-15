package mdns

import (
	"context"
	"fmt"

	"github.com/grandcat/zeroconf"
)

const (
	pairingService = "_adb-tls-pairing._tcp"
	connectService = "_adb-tls-connect._tcp"
	domain         = "local."
)

type Endpoint struct {
	Host string
	Port int
}

// BrowsePairing watches _adb-tls-pairing._tcp until an entry with Instance
// equal to wantInstance appears (or ctx expires). The Android device uses
// the QR's `S:` field verbatim as the instance name, so matching by
// instance avoids picking up a different phone on the same Wi-Fi.
func BrowsePairing(ctx context.Context, wantInstance string) (Endpoint, error) {
	return browseUntil(ctx, pairingService, func(e *zeroconf.ServiceEntry) bool {
		return e.Instance == wantInstance
	})
}

// BrowseConnect watches _adb-tls-connect._tcp and returns the first entry.
// The connect instance name is `adb-<serial>-<rand>`, unknown beforehand;
// since browse is started only after a successful pair, the announce we
// catch belongs to the device we just paired with in the common case.
func BrowseConnect(ctx context.Context) (Endpoint, error) {
	return browseUntil(ctx, connectService, func(*zeroconf.ServiceEntry) bool {
		return true
	})
}

func browseUntil(ctx context.Context, service string, match func(*zeroconf.ServiceEntry) bool) (Endpoint, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return Endpoint{}, fmt.Errorf("mdns resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry, 4)
	browseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := resolver.Browse(browseCtx, service, domain, entries); err != nil {
		return Endpoint{}, fmt.Errorf("mdns browse %s: %w", service, err)
	}

	for {
		select {
		case <-ctx.Done():
			return Endpoint{}, fmt.Errorf("mdns browse %s: %w", service, ctx.Err())
		case e, ok := <-entries:
			if !ok {
				return Endpoint{}, fmt.Errorf("mdns browse %s: channel closed", service)
			}
			if e == nil || !match(e) {
				continue
			}
			host := pickAddr(e)
			if host == "" {
				continue
			}
			return Endpoint{Host: host, Port: e.Port}, nil
		}
	}
}

func pickAddr(e *zeroconf.ServiceEntry) string {
	for _, ip := range e.AddrIPv4 {
		if ip != nil && !ip.IsUnspecified() {
			return ip.String()
		}
	}
	for _, ip := range e.AddrIPv6 {
		if ip != nil && !ip.IsUnspecified() {
			return ip.String()
		}
	}
	return ""
}
