package mdns

import (
	"context"
	"fmt"
	"net"
	"sort"
	"time"

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

// Options tunes how browsing is performed.
type Options struct {
	// Iface restricts browsing to a single network interface, by name.
	// Browsing every interface is the right default, until a VPN, a container
	// bridge, or a second NIC starts swallowing the multicast traffic.
	Iface string
	// Logf, when set, receives every discovered service entry.
	Logf Logf
}

// resolver builds a zeroconf resolver bound to the requested interface.
func (o Options) resolver() (*zeroconf.Resolver, error) {
	if o.Iface == "" {
		return zeroconf.NewResolver()
	}
	iface, err := lookupInterface(o.Iface)
	if err != nil {
		return nil, err
	}
	return zeroconf.NewResolver(zeroconf.SelectIfaces([]net.Interface{iface}))
}

// BrowsePairing watches _adb-tls-pairing._tcp until an entry with Instance
// equal to wantInstance appears (or ctx expires). The Android device uses
// the QR's `S:` field verbatim as the instance name, so matching by
// instance avoids picking up a different phone on the same Wi-Fi.
func BrowsePairing(ctx context.Context, wantInstance string, opts Options) (Endpoint, error) {
	return browseUntil(ctx, pairingService, func(e *zeroconf.ServiceEntry) bool {
		return e.Instance == wantInstance
	}, opts)
}

// BrowseConnect watches _adb-tls-connect._tcp and returns discovered entries.
// The connect instance name is `adb-<serial>-<rand>`, unknown beforehand;
// since browse is started only after a successful pair, the first announces
// usually include the device we just paired with.
func BrowseConnect(ctx context.Context, settle time.Duration, opts Options) ([]Endpoint, error) {
	return browseCandidates(ctx, connectService, func(*zeroconf.ServiceEntry) bool {
		return true
	}, settle, opts)
}

// CheckInterface reports whether an interface name can carry mDNS traffic.
func CheckInterface(name string) error {
	_, err := lookupInterface(name)
	return err
}

// Interface is a network interface that can carry mDNS traffic.
type Interface struct {
	Name string
	IPs  []string
}

// MulticastInterfaces lists the interfaces a device could plausibly be reached
// on — up, multicast-capable, not loopback, and holding a routable address.
// Interfaces with only link-local addresses (VPN tunnels, AWDL) are left out to
// keep the list short; Options.Iface still accepts them if a setup needs one.
func MulticastInterfaces() ([]Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	var out []Interface
	for _, iface := range all {
		if !carriesMulticast(iface) || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var ips []string
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || !isRoutable(ipnet.IP) {
				continue
			}
			ips = append(ips, ipnet.IP.String())
		}
		if len(ips) == 0 {
			continue
		}
		out = append(out, Interface{Name: iface.Name, IPs: ips})
	}
	return out, nil
}

func isRoutable(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// lookupInterface resolves an interface by name and rejects the ones that
// cannot carry mDNS, so a typo or a sleeping interface fails immediately
// instead of looking like a silent discovery timeout.
func lookupInterface(name string) (net.Interface, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return net.Interface{}, fmt.Errorf("interface %q not found: %w", name, err)
	}
	if iface.Flags&net.FlagUp == 0 {
		return net.Interface{}, fmt.Errorf("interface %q is down", name)
	}
	if iface.Flags&net.FlagMulticast == 0 {
		return net.Interface{}, fmt.Errorf("interface %q does not support multicast", name)
	}
	return *iface, nil
}

func carriesMulticast(iface net.Interface) bool {
	return iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0
}

// PreferHost returns endpoints ordered so entries matching preferredHost are
// tried first while preserving discovery order inside each group.
func PreferHost(endpoints []Endpoint, preferredHost string) []Endpoint {
	if preferredHost == "" || len(endpoints) < 2 {
		return endpoints
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].Host == preferredHost && endpoints[j].Host != preferredHost
	})
	return endpoints
}

func browseUntil(ctx context.Context, service string, match func(*zeroconf.ServiceEntry) bool, opts Options) (Endpoint, error) {
	resolver, err := opts.resolver()
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
				logEntry(opts.Logf, service, e)
				continue
			}
			logEntry(opts.Logf, service, e)
			host := pickAddr(e)
			if host == "" {
				continue
			}
			return Endpoint{Host: host, Port: e.Port}, nil
		}
	}
}

func browseCandidates(ctx context.Context, service string, match func(*zeroconf.ServiceEntry) bool, settle time.Duration, opts Options) ([]Endpoint, error) {
	resolver, err := opts.resolver()
	if err != nil {
		return nil, fmt.Errorf("mdns resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry, 8)
	browseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := resolver.Browse(browseCtx, service, domain, entries); err != nil {
		return nil, fmt.Errorf("mdns browse %s: %w", service, err)
	}

	var endpoints []Endpoint
	seen := make(map[Endpoint]bool)
	var settleTimer <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if len(endpoints) > 0 {
				return endpoints, nil
			}
			return nil, fmt.Errorf("mdns browse %s: %w", service, ctx.Err())
		case <-settleTimer:
			return endpoints, nil
		case e, ok := <-entries:
			if !ok {
				if len(endpoints) > 0 {
					return endpoints, nil
				}
				return nil, fmt.Errorf("mdns browse %s: channel closed", service)
			}
			logEntry(opts.Logf, service, e)
			if e == nil || !match(e) {
				continue
			}
			host := pickAddr(e)
			if host == "" {
				continue
			}
			ep := Endpoint{Host: host, Port: e.Port}
			if seen[ep] {
				continue
			}
			seen[ep] = true
			endpoints = append(endpoints, ep)
			if settleTimer == nil {
				settleTimer = time.After(settle)
			}
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
