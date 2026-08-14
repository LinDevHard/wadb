package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lindevhard/wadb/internal/adb"
	"github.com/lindevhard/wadb/internal/mdns"
	"github.com/lindevhard/wadb/internal/pairing"
)

const (
	defaultPairingTimeout = 120 * time.Second
	defaultConnectTimeout = 30 * time.Second
	connectSettleDelay    = 2 * time.Second
)

// version is populated at build time via -ldflags "-X main.version=...".
var version = "dev"

var (
	findADB         = adb.Find
	getADBVersion   = adb.Version
	adbStartServer  = adb.StartServer
	adbMDNSServices = adb.MDNSServices
	adbPair         = adb.Pair
	adbConnect      = adb.Connect
	adbDeviceName   = adb.DeviceName
	browsePairing   = mdns.BrowsePairing
	browseConnect   = mdns.BrowseConnect
)

func main() {
	envOpts, err := loadEnvOptions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	showVersion, options := registerFlags(flag.CommandLine, envOpts)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	opts := options()

	switch {
	case flag.NArg() == 0:
		err = run(opts)
	case flag.NArg() == 1 && flag.Arg(0) == "connect":
		err = connect(opts)
	case flag.NArg() == 1 && flag.Arg(0) == "doctor":
		err = doctor(opts)
	default:
		fmt.Fprintf(os.Stderr, "error: unexpected positional arguments: %v\n\n", flag.Args())
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// registerFlags defines every flag on fs, taking defaults from env, and
// returns the --version flag together with a function that collects the
// parsed values. Keeping the definitions in one place lets the tests parse
// the real flag set instead of a copy of it.
func registerFlags(fs *flag.FlagSet, env runOptions) (showVersion *bool, options func() runOptions) {
	showVersion = fs.Bool("version", false, "print version and exit")
	fs.BoolVar(showVersion, "v", false, "shorthand for --version")
	adbPath := fs.String("adb", env.ADBPath, "path to adb binary (env: WADB_ADB; default: auto-detect)")
	iface := fs.String("iface", env.Iface, "network interface to browse for mDNS (env: WADB_IFACE; default: all)")
	pairOnly := fs.Bool("pair-only", env.PairOnly, "pair the device, then exit without running adb connect (env: WADB_PAIR_ONLY)")
	qrASCII := fs.Bool("qr-ascii", env.QRASCII, "render the QR code with plain ASCII blocks (env: WADB_QR_ASCII)")
	qrInvert := fs.Bool("qr-invert", env.QRInvert, "invert the QR code for terminals with a light background (env: WADB_QR_INVERT)")
	qrSixel := fs.Bool("qr-sixel", env.QRSixel, "render the QR code as a sixel image (env: WADB_QR_SIXEL)")
	verbose := fs.Bool("verbose", env.Verbose, "print discovered mDNS service entries to stderr (env: WADB_VERBOSE)")
	pairingTimeout := fs.Duration("pair-timeout", env.PairingTimeout, "time to wait for the pairing mDNS announce (env: WADB_PAIR_TIMEOUT)")
	connectTimeout := fs.Duration("connect-timeout", env.ConnectTimeout, "time to wait for the connect mDNS announce (env: WADB_CONNECT_TIMEOUT)")

	return showVersion, func() runOptions {
		return runOptions{
			ADBPath:        *adbPath,
			Iface:          *iface,
			PairingTimeout: *pairingTimeout,
			ConnectTimeout: *connectTimeout,
			PairOnly:       *pairOnly,
			QRASCII:        *qrASCII,
			QRInvert:       *qrInvert,
			QRSixel:        *qrSixel,
			Verbose:        *verbose,
		}
	}
}

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprintln(w, "wadb — pair Android devices over ADB Wi-Fi via a terminal QR code.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  wadb [flags]")
	fmt.Fprintln(w, "  wadb [flags] connect")
	fmt.Fprintln(w, "  wadb [flags] doctor")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "With no arguments, wadb prints a QR code. Scan it from")
	fmt.Fprintln(w, "Settings → Developer options → Wireless debugging → Pair device with QR code")
	fmt.Fprintln(w, "on an Android 11+ device sharing the same Wi-Fi network. wadb will")
	fmt.Fprintln(w, "pair and connect automatically, then exit.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  connect  reconnect to a device already paired with this host, without a QR code")
	fmt.Fprintln(w, "  doctor   report the local adb, its version, and mDNS services it can see")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	flag.PrintDefaults()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintln(w, "  WADB_ADB, WADB_IFACE, WADB_PAIR_ONLY, WADB_QR_ASCII, WADB_QR_INVERT, WADB_QR_SIXEL,")
	fmt.Fprintln(w, "  WADB_VERBOSE, WADB_PAIR_TIMEOUT, WADB_CONNECT_TIMEOUT")
	fmt.Fprintln(w, "  CLI flags override environment values.")
}

func doctor(opts runOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	adbPath := opts.ADBPath
	if adbPath == "" {
		found, err := findADB()
		if err != nil {
			return err
		}
		adbPath = found
	}
	fmt.Println("adb:", adbPath)

	adbVersion, err := getADBVersion(ctx, adbPath)
	if err != nil {
		fmt.Println("adb version: warning:", err)
	} else if adbVersion.PlatformToolsMajor > 0 {
		fmt.Println("platform-tools:", adbVersion.PlatformToolsMajor)
		if !adbVersion.SupportsWifi2Improvements() {
			fmt.Printf("warning: platform-tools before %d may miss newer ADB Wi-Fi mDNS and reconnect improvements.\n", adb.Wifi2PlatformToolsMajor)
		}
	} else {
		fmt.Println("platform-tools: unknown")
		if opts.Verbose {
			fmt.Println(adbVersion.Raw)
		}
	}

	reportInterfaces(opts.Iface)

	if err := adbStartServer(ctx, adbPath); err != nil {
		return err
	}
	fmt.Println("adb server: running")

	services, err := adbMDNSServices(ctx, adbPath)
	if err != nil {
		fmt.Println("mDNS services: warning:", err)
		fmt.Println("hint: if pairing hangs, check same Wi-Fi, AP isolation, firewall rules, and UDP 5353.")
		return nil
	}
	serviceLines := adb.ParseMDNSServices(services)
	if len(serviceLines) == 0 {
		fmt.Println("mDNS services: none reported by adb")
		fmt.Println("hint: this is normal when no Android device is advertising Wireless debugging right now.")
		return nil
	}
	fmt.Println("mDNS services:")
	fmt.Println(strings.Join(serviceLines, "\n"))
	return nil
}

// reportInterfaces validates an explicit --iface, or lists the interfaces
// worth passing to it. Discovery failures are usually a multicast routing
// problem, so knowing which interfaces exist is half the diagnosis.
func reportInterfaces(iface string) {
	if iface != "" {
		if err := mdns.CheckInterface(iface); err != nil {
			fmt.Println("interface: error:", err)
			return
		}
		fmt.Println("interface:", iface)
		return
	}

	ifaces, err := mdns.MulticastInterfaces()
	if err != nil {
		fmt.Println("interfaces: warning:", err)
		return
	}
	if len(ifaces) == 0 {
		fmt.Println("interfaces: none are up and multicast-capable")
		return
	}
	fmt.Println("interfaces (candidates for --iface):")
	for _, i := range ifaces {
		fmt.Printf("  %s %s\n", i.Name, strings.Join(i.IPs, " "))
	}
}

type runOptions struct {
	ADBPath        string
	Iface          string
	PairingTimeout time.Duration
	ConnectTimeout time.Duration
	PairOnly       bool
	QRASCII        bool
	QRInvert       bool
	QRSixel        bool
	Verbose        bool
}

// mdnsOptions builds the discovery options shared by the pair and connect
// flows, rejecting an unusable --iface here rather than letting it surface
// later as a discovery timeout that blames the network.
func (o runOptions) mdnsOptions() (mdns.Options, error) {
	if o.Iface != "" {
		if err := mdns.CheckInterface(o.Iface); err != nil {
			return mdns.Options{}, err
		}
	}
	opts := mdns.Options{Iface: o.Iface}
	if o.Verbose {
		opts.Logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}
	return opts, nil
}

func loadEnvOptions() (runOptions, error) {
	opts := runOptions{
		PairingTimeout: defaultPairingTimeout,
		ConnectTimeout: defaultConnectTimeout,
	}

	opts.ADBPath = strings.TrimSpace(os.Getenv("WADB_ADB"))
	opts.Iface = strings.TrimSpace(os.Getenv("WADB_IFACE"))

	var err error
	if opts.PairOnly, err = envBool("WADB_PAIR_ONLY", opts.PairOnly); err != nil {
		return runOptions{}, err
	}
	if opts.QRASCII, err = envBool("WADB_QR_ASCII", opts.QRASCII); err != nil {
		return runOptions{}, err
	}
	if opts.QRInvert, err = envBool("WADB_QR_INVERT", opts.QRInvert); err != nil {
		return runOptions{}, err
	}
	if opts.QRSixel, err = envBool("WADB_QR_SIXEL", opts.QRSixel); err != nil {
		return runOptions{}, err
	}
	if opts.Verbose, err = envBool("WADB_VERBOSE", opts.Verbose); err != nil {
		return runOptions{}, err
	}
	if opts.PairingTimeout, err = envDuration("WADB_PAIR_TIMEOUT", opts.PairingTimeout); err != nil {
		return runOptions{}, err
	}
	if opts.ConnectTimeout, err = envDuration("WADB_CONNECT_TIMEOUT", opts.ConnectTimeout); err != nil {
		return runOptions{}, err
	}

	return opts, nil
}

func envBool(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %q", name, raw)
	}
	return value, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration like 30s or 3m: %q", name, raw)
	}
	return value, nil
}

// setupADB resolves the adb binary, reports its platform-tools version, and
// makes sure the daemon is running before any mDNS discovery starts.
func setupADB(ctx context.Context, opts runOptions) (string, error) {
	adbPath := opts.ADBPath
	if adbPath == "" {
		found, err := findADB()
		if err != nil {
			return "", err
		}
		adbPath = found
	}
	fmt.Fprintln(os.Stderr, "Using adb:", adbPath)

	adbVersion, err := getADBVersion(ctx, adbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	} else if adbVersion.PlatformToolsMajor > 0 {
		fmt.Fprintf(os.Stderr, "Using platform-tools: %d\n", adbVersion.PlatformToolsMajor)
		if !adbVersion.SupportsWifi2Improvements() {
			fmt.Fprintf(os.Stderr, "Warning: platform-tools before %d may miss newer ADB Wi-Fi mDNS and reconnect improvements.\n", adb.Wifi2PlatformToolsMajor)
		}
	} else if opts.Verbose {
		fmt.Fprintln(os.Stderr, "adb version output:")
		fmt.Fprintln(os.Stderr, adbVersion.Raw)
	}

	if err := adbStartServer(ctx, adbPath); err != nil {
		return "", err
	}
	return adbPath, nil
}

// discoverConnectEndpoints waits for _adb-tls-connect._tcp announces and
// returns them ordered so preferredHost, when known, is tried first. Callers
// wrap the error with a hint that fits their flow.
func discoverConnectEndpoints(ctx context.Context, adbPath string, timeout time.Duration, preferredHost string, opts mdns.Options) ([]mdns.Endpoint, error) {
	connCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoints, err := browseConnect(connCtx, connectSettleDelay, opts)
	if err != nil {
		endpoints = adbReportedConnectEndpoints(ctx, adbPath, opts.Logf)
		if len(endpoints) == 0 {
			return nil, err
		}
		fmt.Fprintln(os.Stderr, "mDNS browse found nothing; using the endpoints adb reports instead.")
	}
	return mdns.PreferHost(endpoints, preferredHost), nil
}

// adbReportedConnectEndpoints asks adb which services it has discovered. adb
// runs its own mDNS implementation and keeps announces cached from before wadb
// started, so it regularly sees a device that our browse just missed. Failures
// are swallowed: this only ever runs as a second chance after discovery
// already failed.
func adbReportedConnectEndpoints(ctx context.Context, adbPath string, logf mdns.Logf) []mdns.Endpoint {
	raw, err := adbMDNSServices(ctx, adbPath)
	if err != nil {
		if logf != nil {
			logf("adb mdns services failed: %v", err)
		}
		return nil
	}

	var endpoints []mdns.Endpoint
	for _, service := range adb.ParseMDNSServiceEntries(raw) {
		if service.Service != mdns.ConnectService {
			continue
		}
		if logf != nil {
			logf("adb mdns services: instance=%q host=%q port=%d", service.Instance, service.Host, service.Port)
		}
		endpoints = append(endpoints, mdns.Endpoint{Host: service.Host, Port: service.Port})
	}
	return endpoints
}

// connectToEndpoints runs adb connect against each endpoint in order and stops
// at the first success. Endpoints belonging to devices this host has not paired
// with simply fail, so trying them all is how the paired one is found.
func connectToEndpoints(ctx context.Context, adbPath string, endpoints []mdns.Endpoint) error {
	var failures []string
	for _, ep := range endpoints {
		fmt.Printf("Connecting to %s:%d...\n", ep.Host, ep.Port)
		out, err := adbConnect(ctx, adbPath, ep.Host, ep.Port)
		if err == nil {
			fmt.Println(out)
			addr := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			if name, err := adbDeviceName(ctx, adbPath, addr); err == nil && name != "" {
				fmt.Println("Device:", name)
			}
			return nil
		}
		failures = append(failures, err.Error())
	}
	return fmt.Errorf("failed to connect to %d discovered endpoint(s): %s", len(endpoints), strings.Join(failures, "; "))
}

// connect reconnects to a device that is already paired with this host. Pairing
// survives reboots and Wi-Fi changes, but the device's port does not, so the
// only thing needed is to discover the current _adb-tls-connect._tcp endpoint.
func connect(opts runOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mdnsOpts, err := opts.mdnsOptions()
	if err != nil {
		return err
	}

	adbPath, err := setupADB(ctx, opts)
	if err != nil {
		return err
	}

	fmt.Println("Looking for devices announcing _adb-tls-connect._tcp...")
	endpoints, err := discoverConnectEndpoints(ctx, adbPath, opts.ConnectTimeout, "", mdnsOpts)
	if err != nil {
		return fmt.Errorf("no _adb-tls-connect._tcp announce appeared within %s: %w\nenable Wireless debugging on a device already paired with this host, or run wadb without arguments to pair a new one\nif a VPN or container bridge is active, limit discovery with --iface (wadb doctor lists the candidates)", opts.ConnectTimeout, err)
	}

	return connectToEndpoints(ctx, adbPath, endpoints)
}

func run(opts runOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mdnsOpts, err := opts.mdnsOptions()
	if err != nil {
		return err
	}

	adbPath, err := setupADB(ctx, opts)
	if err != nil {
		return err
	}

	serviceName, err := pairing.GenerateServiceName()
	if err != nil {
		return err
	}
	password, err := pairing.GeneratePassword()
	if err != nil {
		return err
	}

	payload := pairing.QRPayload(serviceName, password)
	fmt.Println()
	fmt.Println("On your Android device:")
	fmt.Println("  Settings → Developer options → Wireless debugging → Pair device with QR code")
	fmt.Println("Then scan the QR below.")
	fmt.Println()
	pairing.RenderQR(os.Stdout, payload, pairing.QROptions{
		ASCII:  opts.QRASCII,
		Invert: opts.QRInvert,
		Sixel:  opts.QRSixel,
	})
	fmt.Println()
	fmt.Println("Waiting for pairing announce...")

	pairCtx, cancelPair := context.WithTimeout(ctx, opts.PairingTimeout)
	defer cancelPair()
	pairEP, err := browsePairing(pairCtx, serviceName, mdnsOpts)
	if err != nil {
		return fmt.Errorf("did not see _adb-tls-pairing._tcp announce within %s: %w\ncheck that both devices are on the same Wi-Fi, Wireless debugging is enabled, and mDNS/UDP 5353 is not blocked by the network or firewall\nif a VPN or container bridge is active, limit discovery with --iface (wadb doctor lists the candidates)", opts.PairingTimeout, err)
	}
	fmt.Printf("Found pairing endpoint %s:%d, pairing...\n", pairEP.Host, pairEP.Port)

	if err := adbPair(ctx, adbPath, pairEP.Host, pairEP.Port, password); err != nil {
		return err
	}
	fmt.Println("Paired successfully.")
	if opts.PairOnly {
		fmt.Println("Pair-only mode enabled; skipping adb connect.")
		return nil
	}

	fmt.Println("Waiting for device to announce on _adb-tls-connect._tcp...")
	connEPs, err := discoverConnectEndpoints(ctx, adbPath, opts.ConnectTimeout, pairEP.Host, mdnsOpts)
	if err != nil {
		return fmt.Errorf("paired successfully, but no _adb-tls-connect._tcp announce appeared within %s: %w\nsome Android builds delay this announce; retry with wadb connect, or run adb connect manually using the host and port shown in Wireless debugging", opts.ConnectTimeout, err)
	}

	return connectToEndpoints(ctx, adbPath, connEPs)
}
