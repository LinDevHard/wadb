package mdns

import (
	"context"
	"fmt"
	"net"

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

type Logf func(format string, args ...any)

// BrowsePairing watches _adb-tls-pairing._tcp until an entry with Instance
// equal to wantInstance appears (or ctx expires). The Android device uses
// the QR's `S:` field verbatim as the instance name, so matching by
// instance avoids picking up a different phone on the same Wi-Fi.
func BrowsePairing(ctx context.Context, wantInstance string, logf Logf) (Endpoint, error) {
	return browseUntil(ctx, pairingService, func(e *zeroconf.ServiceEntry) bool {
		return e.Instance == wantInstance
	}, logf)
}

// BrowseConnect watches _adb-tls-connect._tcp and returns the first entry.
// The connect instance name is `adb-<serial>-<rand>`, unknown beforehand;
// since browse is started only after a successful pair, the announce we
// catch belongs to the device we just paired with in the common case.
func BrowseConnect(ctx context.Context, logf Logf) (Endpoint, error) {
	return browseUntil(ctx, connectService, func(*zeroconf.ServiceEntry) bool {
		return true
	}, logf)
}

func browseUntil(ctx context.Context, service string, match func(*zeroconf.ServiceEntry) bool, logf Logf) (Endpoint, error) {
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
				logEntry(logf, service, e)
				continue
			}
			logEntry(logf, service, e)
			host := pickAddr(e)
			if host == "" {
				continue
			}
			return Endpoint{Host: host, Port: e.Port}, nil
		}
	}
}

func logEntry(logf Logf, service string, e *zeroconf.ServiceEntry) {
	if logf == nil || e == nil {
		return
	}
	logf(
		"mDNS %s: instance=%q host=%q port=%d ipv4=%v ipv6=%v txt=%q",
		service,
		e.Instance,
		e.HostName,
		e.Port,
		ips(e.AddrIPv4),
		ips(e.AddrIPv6),
		e.Text,
	)
}

func ips(addrs []net.IP) []string {
	out := make([]string, 0, len(addrs))
	for _, ip := range addrs {
		if ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
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
