package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lindevhard/wadb/internal/adb"
	"github.com/lindevhard/wadb/internal/mdns"
	"github.com/lindevhard/wadb/internal/pairing"
	"golang.org/x/term"
)

const (
	defaultPairingTimeout = 120 * time.Second
	defaultConnectTimeout = 30 * time.Second
	defaultScanTimeout    = 3 * time.Second
	connectSettleDelay    = 2 * time.Second
)

// version is populated at build time via -ldflags "-X main.version=...".
var version = "dev"

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

var (
	findADB         = adb.Find
	getADBVersion   = adb.Version
	adbStartServer  = adb.StartServer
	adbMDNSServices = adb.MDNSServices
	adbDevices      = adb.Devices
	adbDisconnect   = adb.Disconnect
	adbPair         = adb.Pair
	adbConnect      = adb.Connect
	adbDeviceName   = adb.DeviceName
	browsePairing   = mdns.BrowsePairing
	browseConnect   = mdns.BrowseConnect
	readPairingCode = promptPairingCode
)

func main() {
	envOpts, err := loadEnvOptions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	showVersion, options := registerFlags(flag.CommandLine, envOpts)
	flag.Usage = usage
	normalizedArgs, err := normalizeCLIArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if err := flag.CommandLine.Parse(normalizedArgs); err != nil {
		os.Exit(2)
	}

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
	case flag.NArg() == 1 && flag.Arg(0) == "devices":
		err = devices(opts)
	case flag.NArg() == 1 && flag.Arg(0) == "disconnect":
		err = disconnect(opts)
	case flag.NArg() == 2 && flag.Arg(0) == "pair":
		err = pairByCode(opts, flag.Arg(1))
	case flag.NArg() == 1 && flag.Arg(0) == "doctor":
		err = doctor(opts)
	default:
		fmt.Fprintf(os.Stderr, "error: unexpected positional arguments: %v\n\n", flag.Args())
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exitCode := 1
		var invalidUsage usageError
		if errors.As(err, &invalidUsage) {
			exitCode = 2
		}
		os.Exit(exitCode)
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
	scanTimeout := fs.Duration("scan-timeout", env.ScanTimeout, "time to scan for devices (env: WADB_SCAN_TIMEOUT)")
	device := fs.String("device", env.Device, "device serial, address, host, or mDNS instance (env: WADB_DEVICE)")
	all := fs.Bool("all", env.All, "operate on every matching wireless device (env: WADB_ALL)")
	jsonOutput := fs.Bool("json", env.JSON, "write machine-readable JSON output (env: WADB_JSON)")

	return showVersion, func() runOptions {
		return runOptions{
			ADBPath:        *adbPath,
			Iface:          *iface,
			PairingTimeout: *pairingTimeout,
			ConnectTimeout: *connectTimeout,
			ScanTimeout:    *scanTimeout,
			Device:         *device,
			All:            *all,
			JSON:           *jsonOutput,
			PairOnly:       *pairOnly,
			QRASCII:        *qrASCII,
			QRInvert:       *qrInvert,
			QRSixel:        *qrSixel,
			Verbose:        *verbose,
		}
	}
}

func normalizeCLIArgs(args []string) ([]string, error) {
	valueFlags := map[string]bool{
		"adb": true, "iface": true, "pair-timeout": true, "connect-timeout": true,
		"scan-timeout": true, "device": true,
	}
	var options, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		options = append(options, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if valueFlags[name] {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag --%s requires a value", name)
			}
			i++
			options = append(options, args[i])
		}
	}
	return append(options, positional...), nil
}

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprintln(w, "wadb — pair Android devices over ADB Wi-Fi via a terminal QR code or pairing code.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  wadb [flags]")
	fmt.Fprintln(w, "  wadb [flags] pair <host:port>")
	fmt.Fprintln(w, "  wadb [flags] connect")
	fmt.Fprintln(w, "  wadb [flags] devices")
	fmt.Fprintln(w, "  wadb [flags] disconnect")
	fmt.Fprintln(w, "  wadb [flags] doctor")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "With no arguments, wadb prints a QR code. Scan it from")
	fmt.Fprintln(w, "Settings → Developer options → Wireless debugging → Pair device with QR code")
	fmt.Fprintln(w, "on an Android 11+ device sharing the same Wi-Fi network. wadb will")
	fmt.Fprintln(w, "pair and connect automatically, then exit.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  pair     pair using the address and six-digit code shown by the device")
	fmt.Fprintln(w, "  connect  reconnect to a device already paired with this host, without a QR code")
	fmt.Fprintln(w, "  devices  list devices known to adb and wireless debugging announces")
	fmt.Fprintln(w, "  disconnect  disconnect one or all wireless devices")
	fmt.Fprintln(w, "  doctor   report the local adb, its version, and mDNS services it can see")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	flag.PrintDefaults()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintln(w, "  WADB_ADB, WADB_IFACE, WADB_PAIR_ONLY, WADB_QR_ASCII, WADB_QR_INVERT, WADB_QR_SIXEL,")
	fmt.Fprintln(w, "  WADB_VERBOSE, WADB_PAIR_TIMEOUT, WADB_CONNECT_TIMEOUT, WADB_SCAN_TIMEOUT,")
	fmt.Fprintln(w, "  WADB_DEVICE, WADB_ALL, WADB_JSON")
	fmt.Fprintln(w, "  CLI flags override environment values.")
}

func promptPairingCode() (string, error) {
	fmt.Fprint(os.Stderr, "Enter pairing code: ")

	var raw []byte
	var err error
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
	} else {
		line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
		raw = []byte(line)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			err = readErr
		}
	}
	if err != nil {
		return "", fmt.Errorf("read pairing code: %w", err)
	}

	code := strings.TrimSpace(string(raw))
	if err := validatePairingCode(code); err != nil {
		return "", err
	}
	return code, nil
}

func validatePairingCode(code string) error {
	if len(code) != 6 {
		return errors.New("pairing code must contain exactly six digits")
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return errors.New("pairing code must contain exactly six digits")
		}
	}
	return nil
}

func parseEndpoint(address string) (mdns.Endpoint, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return mdns.Endpoint{}, fmt.Errorf("invalid pairing address %q: expected host:port (IPv6 addresses need brackets): %w", address, err)
	}
	if host == "" {
		return mdns.Endpoint{}, fmt.Errorf("invalid pairing address %q: host is empty", address)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return mdns.Endpoint{}, fmt.Errorf("invalid pairing address %q: port must be between 1 and 65535", address)
	}
	return mdns.Endpoint{Host: host, Port: port}, nil
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
	ScanTimeout    time.Duration
	Device         string
	All            bool
	JSON           bool
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
		ScanTimeout:    defaultScanTimeout,
	}

	opts.ADBPath = strings.TrimSpace(os.Getenv("WADB_ADB"))
	opts.Iface = strings.TrimSpace(os.Getenv("WADB_IFACE"))
	opts.Device = strings.TrimSpace(os.Getenv("WADB_DEVICE"))

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
	if opts.ScanTimeout, err = envDuration("WADB_SCAN_TIMEOUT", opts.ScanTimeout); err != nil {
		return runOptions{}, err
	}
	if opts.All, err = envBool("WADB_ALL", opts.All); err != nil {
		return runOptions{}, err
	}
	if opts.JSON, err = envBool("WADB_JSON", opts.JSON); err != nil {
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
	endpoints, err := scanConnectEndpoints(ctx, adbPath, timeout, opts)
	if err != nil && len(endpoints) == 0 {
		return nil, err
	}
	return mdns.PreferHost(endpoints, preferredHost), nil
}

func scanConnectEndpoints(ctx context.Context, adbPath string, timeout time.Duration, opts mdns.Options) ([]mdns.Endpoint, error) {
	cached, cacheErr := adbReportedConnectEndpoints(ctx, adbPath, opts.Logf)
	liveTimeout := timeout
	if len(cached) > 0 && liveTimeout > defaultScanTimeout {
		liveTimeout = defaultScanTimeout
	}
	connCtx, cancel := context.WithTimeout(ctx, liveTimeout)
	defer cancel()
	live, browseErr := browseConnect(connCtx, connectSettleDelay, opts)

	endpoints := mergeEndpoints(cached, live)
	if len(endpoints) > 0 {
		return endpoints, nil
	}
	if browseErr != nil && cacheErr != nil {
		return nil, errors.Join(browseErr, cacheErr)
	}
	if browseErr != nil {
		return nil, browseErr
	}
	if cacheErr != nil {
		return nil, cacheErr
	}
	return nil, errors.New("no _adb-tls-connect._tcp endpoints found")
}

func mergeEndpoints(groups ...[]mdns.Endpoint) []mdns.Endpoint {
	var out []mdns.Endpoint
	byAddress := make(map[string]int)
	for _, endpoints := range groups {
		for _, endpoint := range endpoints {
			address := net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))
			if i, ok := byAddress[address]; ok {
				if endpoint.Instance != "" {
					out[i].Instance = endpoint.Instance
				}
				continue
			}
			byAddress[address] = len(out)
			out = append(out, endpoint)
		}
	}
	return out
}

// adbReportedConnectEndpoints asks adb which services it has discovered. adb
// runs its own mDNS implementation and keeps announces cached from before wadb
// started, so it regularly sees a device that our browse just missed.
func adbReportedConnectEndpoints(ctx context.Context, adbPath string, logf mdns.Logf) ([]mdns.Endpoint, error) {
	raw, err := adbMDNSServices(ctx, adbPath)
	if err != nil {
		if logf != nil {
			logf("adb mdns services failed: %v", err)
		}
		return nil, err
	}

	var endpoints []mdns.Endpoint
	for _, service := range adb.ParseMDNSServiceEntries(raw) {
		if service.Service != mdns.ConnectService {
			continue
		}
		if logf != nil {
			logf("adb mdns services: instance=%q host=%q port=%d", service.Instance, service.Host, service.Port)
		}
		endpoints = append(endpoints, mdns.Endpoint{Instance: service.Instance, Host: service.Host, Port: service.Port})
	}
	return endpoints, nil
}

// connectToEndpoints runs adb connect against each endpoint in order and stops
// at the first success. Endpoints belonging to devices this host has not paired
// with simply fail, so trying them all is how the paired one is found.
func connectToEndpoints(ctx context.Context, adbPath string, endpoints []mdns.Endpoint) error {
	return connectToEndpointsMode(ctx, adbPath, endpoints, false, false)
}

type connectionResult struct {
	Address  string `json:"address"`
	Instance string `json:"instance,omitempty"`
	Status   string `json:"status"`
	Device   string `json:"device,omitempty"`
	Error    string `json:"error,omitempty"`
}

func connectToEndpointsMode(ctx context.Context, adbPath string, endpoints []mdns.Endpoint, all, jsonOutput bool) error {
	var failures []string
	var results []connectionResult
	succeeded := 0
	for _, ep := range endpoints {
		addr := net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port))
		if !jsonOutput {
			fmt.Printf("Connecting to %s...\n", addr)
		}
		out, err := adbConnect(ctx, adbPath, ep.Host, ep.Port)
		if err == nil {
			result := connectionResult{Address: addr, Instance: ep.Instance, Status: "connected"}
			if name, nameErr := adbDeviceName(ctx, adbPath, addr); nameErr == nil && name != "" {
				result.Device = name
			}
			results = append(results, result)
			succeeded++
			if !jsonOutput {
				fmt.Println(out)
				if result.Device != "" {
					fmt.Println("Device:", result.Device)
				}
			}
			if !all {
				if jsonOutput {
					if encodeErr := json.NewEncoder(os.Stdout).Encode(results); encodeErr != nil {
						return fmt.Errorf("write JSON: %w", encodeErr)
					}
				}
				return nil
			}
			continue
		}
		failures = append(failures, err.Error())
		results = append(results, connectionResult{Address: addr, Instance: ep.Instance, Status: "failed", Error: err.Error()})
		if all && !jsonOutput {
			fmt.Fprintln(os.Stderr, "Warning:", err)
		}
	}
	if jsonOutput {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(results); encodeErr != nil {
			return fmt.Errorf("write JSON: %w", encodeErr)
		}
	}
	if succeeded > 0 {
		return nil
	}
	return fmt.Errorf("failed to connect to %d discovered endpoint(s): %s", len(endpoints), strings.Join(failures, "; "))
}

func endpointID(instance string) string {
	if !strings.HasPrefix(instance, "adb-") {
		return ""
	}
	trimmed := strings.TrimPrefix(instance, "adb-")
	if i := strings.LastIndexByte(trimmed, '-'); i > 0 {
		return trimmed[:i]
	}
	return trimmed
}

func selectEndpoints(endpoints []mdns.Endpoint, selector string) []mdns.Endpoint {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return endpoints
	}
	var selected []mdns.Endpoint
	for _, endpoint := range endpoints {
		address := net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))
		for _, candidate := range []string{endpointID(endpoint.Instance), endpoint.Instance, address, endpoint.Host} {
			if strings.EqualFold(selector, candidate) {
				selected = append(selected, endpoint)
				break
			}
		}
	}
	return selected
}

// connect reconnects to a device that is already paired with this host. Pairing
// survives reboots and Wi-Fi changes, but the device's port does not, so the
// only thing needed is to discover the current _adb-tls-connect._tcp endpoint.
func connect(opts runOptions) error {
	if opts.All && opts.Device != "" {
		return usageError{errors.New("--all and --device cannot be used together")}
	}
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
	if opts.Device != "" {
		if endpoint, parseErr := parseEndpoint(opts.Device); parseErr == nil {
			return connectToEndpointsMode(ctx, adbPath, []mdns.Endpoint{endpoint}, false, opts.JSON)
		}
	}

	if !opts.JSON {
		fmt.Println("Looking for devices announcing _adb-tls-connect._tcp...")
	}
	endpoints, err := discoverConnectEndpoints(ctx, adbPath, opts.ConnectTimeout, "", mdnsOpts)
	if err != nil {
		return fmt.Errorf("no _adb-tls-connect._tcp announce appeared within %s: %w\nenable Wireless debugging on a device already paired with this host, or run wadb without arguments to pair a new one\nif a VPN or container bridge is active, limit discovery with --iface (wadb doctor lists the candidates)", opts.ConnectTimeout, err)
	}

	endpoints = selectEndpoints(endpoints, opts.Device)
	if len(endpoints) == 0 {
		return fmt.Errorf("no discovered device matches %q", opts.Device)
	}
	return connectToEndpointsMode(ctx, adbPath, endpoints, opts.All, opts.JSON)
}

type managedDevice struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	Status    string `json:"status"`
	Name      string `json:"name,omitempty"`
	Transport string `json:"transport"`
	Source    string `json:"source"`
	Instance  string `json:"instance,omitempty"`
}

func devices(opts runOptions) error {
	if opts.All {
		return usageError{errors.New("--all is not used by the devices command")}
	}
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

	connected, err := adbDevices(ctx, adbPath)
	if err != nil {
		return err
	}
	announced, scanErr := scanConnectEndpoints(ctx, adbPath, opts.ScanTimeout, mdnsOpts)
	if scanErr != nil && opts.Verbose {
		fmt.Fprintln(os.Stderr, "Warning: device scan:", scanErr)
	}
	rows := mergeManagedDevices(connected, announced)
	rows = selectManagedDevices(rows, opts.Device)
	if opts.JSON {
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Println("No ADB devices or wireless debugging announces found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tADDRESS\tSTATUS\tTRANSPORT\tDEVICE\tMDNS INSTANCE")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", row.ID, row.Address, row.Status, row.Transport, row.Name, row.Instance)
	}
	return w.Flush()
}

func mergeManagedDevices(connected []adb.DeviceEntry, announced []mdns.Endpoint) []managedDevice {
	rows := make([]managedDevice, 0, len(connected)+len(announced))
	byAddress := make(map[string]int)
	for _, device := range connected {
		name := strings.ReplaceAll(device.Model, "_", " ")
		transport := "usb"
		id := device.Serial
		if strings.HasPrefix(device.Serial, "emulator-") {
			transport = "emulator"
		} else if _, _, err := net.SplitHostPort(device.Serial); err == nil {
			transport = "wifi"
		}
		status := device.State
		if status == "device" {
			status = "connected"
		}
		rows = append(rows, managedDevice{
			ID:        id,
			Address:   device.Serial,
			Status:    status,
			Name:      name,
			Transport: transport,
			Source:    "adb",
		})
		byAddress[device.Serial] = len(rows) - 1
	}
	for _, endpoint := range announced {
		address := net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))
		if i, ok := byAddress[address]; ok {
			rows[i].Instance = endpoint.Instance
			rows[i].Source = "adb+mdns"
			if id := endpointID(endpoint.Instance); id != "" {
				rows[i].ID = id
			}
			continue
		}
		id := endpointID(endpoint.Instance)
		if id == "" {
			id = address
		}
		rows = append(rows, managedDevice{
			ID:        id,
			Address:   address,
			Status:    "discovered",
			Transport: "wifi",
			Source:    "mdns",
			Instance:  endpoint.Instance,
		})
		byAddress[address] = len(rows) - 1
	}
	return consolidateManagedDevices(rows)
}

func consolidateManagedDevices(rows []managedDevice) []managedDevice {
	out := make([]managedDevice, 0, len(rows))
	byID := make(map[string]int)
	for _, row := range rows {
		key := strings.ToLower(row.ID)
		if i, ok := byID[key]; ok && key != "" {
			out[i].Address = appendCSV(out[i].Address, row.Address)
			out[i].Transport = appendCSV(out[i].Transport, row.Transport)
			out[i].Source = appendCSV(out[i].Source, row.Source)
			if out[i].Name == "" {
				out[i].Name = row.Name
			}
			if out[i].Instance == "" {
				out[i].Instance = row.Instance
			}
			if row.Status == "connected" {
				out[i].Status = row.Status
			}
			continue
		}
		byID[key] = len(out)
		out = append(out, row)
	}
	return out
}

func appendCSV(current, value string) string {
	if value == "" {
		return current
	}
	for _, existing := range strings.Split(current, ",") {
		if strings.TrimSpace(existing) == value {
			return current
		}
	}
	if current == "" {
		return value
	}
	return current + "," + value
}

func selectManagedDevices(rows []managedDevice, selector string) []managedDevice {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return rows
	}
	selected := make([]managedDevice, 0)
	for _, row := range rows {
		candidates := []string{row.ID, row.Instance, row.Name}
		candidates = append(candidates, strings.Split(row.Address, ",")...)
		for _, candidate := range candidates {
			if strings.EqualFold(selector, candidate) {
				selected = append(selected, row)
				break
			}
		}
	}
	return selected
}

type disconnectResult struct {
	Address string `json:"address"`
	Status  string `json:"status"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

func disconnect(opts runOptions) error {
	if opts.All && opts.Device != "" {
		return usageError{errors.New("--all and --device cannot be used together")}
	}
	if !opts.All && opts.Device == "" {
		return usageError{errors.New("disconnect requires --device <id|address> or --all")}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	adbPath, err := setupADB(ctx, opts)
	if err != nil {
		return err
	}
	if opts.All {
		out, err := adbDisconnect(ctx, adbPath, "")
		result := disconnectResult{Address: "all", Status: "disconnected", Output: out}
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
		}
		if opts.JSON {
			if encodeErr := json.NewEncoder(os.Stdout).Encode([]disconnectResult{result}); encodeErr != nil {
				return fmt.Errorf("write JSON: %w", encodeErr)
			}
		} else if out != "" {
			fmt.Println(out)
		}
		return err
	}

	addresses, err := resolveDisconnectAddresses(ctx, adbPath, opts)
	if err != nil {
		return err
	}
	var results []disconnectResult
	var failures []string
	for _, address := range addresses {
		out, disconnectErr := adbDisconnect(ctx, adbPath, address)
		result := disconnectResult{Address: address, Status: "disconnected", Output: out}
		if disconnectErr != nil {
			result.Status = "failed"
			result.Error = disconnectErr.Error()
			failures = append(failures, disconnectErr.Error())
		} else if !opts.JSON && out != "" {
			fmt.Println(out)
		}
		results = append(results, result)
	}
	if opts.JSON {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(results); encodeErr != nil {
			return fmt.Errorf("write JSON: %w", encodeErr)
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func resolveDisconnectAddresses(ctx context.Context, adbPath string, opts runOptions) ([]string, error) {
	if _, _, err := net.SplitHostPort(opts.Device); err == nil {
		return []string{opts.Device}, nil
	}
	mdnsOpts, err := opts.mdnsOptions()
	if err != nil {
		return nil, err
	}
	endpoints, scanErr := scanConnectEndpoints(ctx, adbPath, opts.ScanTimeout, mdnsOpts)
	selected := selectEndpoints(endpoints, opts.Device)
	var addresses []string
	for _, endpoint := range selected {
		addresses = append(addresses, net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)))
	}
	connected, listErr := adbDevices(ctx, adbPath)
	if listErr == nil {
		for _, device := range connected {
			if _, _, splitErr := net.SplitHostPort(device.Serial); splitErr == nil && strings.EqualFold(device.Serial, opts.Device) {
				addresses = append(addresses, device.Serial)
			}
		}
	}
	addresses = uniqueStrings(addresses)
	if len(addresses) == 0 {
		if scanErr != nil {
			return nil, fmt.Errorf("no wireless device matches %q; discovery failed: %w", opts.Device, scanErr)
		}
		return nil, fmt.Errorf("no wireless device matches %q", opts.Device)
	}
	return addresses, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// pairByCode pairs with the address and one-time code shown by Android's
// "Pair device with pairing code" dialog. The pairing endpoint is not the
// endpoint used for adb connections, so a successful pair is followed by the
// same mDNS connect discovery as the QR flow.
func pairByCode(opts runOptions, address string) error {
	pairEP, err := parseEndpoint(address)
	if err != nil {
		return usageError{err}
	}

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

	code, err := readPairingCode()
	if err != nil {
		return err
	}

	fmt.Printf("Pairing with %s...\n", net.JoinHostPort(pairEP.Host, strconv.Itoa(pairEP.Port)))
	if err := adbPair(ctx, adbPath, pairEP.Host, pairEP.Port, code); err != nil {
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
		return fmt.Errorf("paired successfully, but no _adb-tls-connect._tcp announce appeared within %s: %w\nretry with wadb connect, or run adb connect manually using the host and port shown in Wireless debugging", opts.ConnectTimeout, err)
	}
	return connectToEndpoints(ctx, adbPath, connEPs)
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
