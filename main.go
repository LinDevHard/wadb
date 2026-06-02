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
	browsePairing   = mdns.BrowsePairing
	browseConnect   = mdns.BrowseConnect
)

func main() {
	envOpts, err := loadEnvOptions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "shorthand for --version")
	adbPath := flag.String("adb", envOpts.ADBPath, "path to adb binary (env: WADB_ADB; default: auto-detect)")
	pairOnly := flag.Bool("pair-only", envOpts.PairOnly, "pair the device, then exit without running adb connect (env: WADB_PAIR_ONLY)")
	qrASCII := flag.Bool("qr-ascii", envOpts.QRASCII, "render the QR code with plain ASCII blocks (env: WADB_QR_ASCII)")
	verbose := flag.Bool("verbose", envOpts.Verbose, "print discovered mDNS service entries to stderr (env: WADB_VERBOSE)")
	pairingTimeout := flag.Duration("pair-timeout", envOpts.PairingTimeout, "time to wait for the pairing mDNS announce (env: WADB_PAIR_TIMEOUT)")
	connectTimeout := flag.Duration("connect-timeout", envOpts.ConnectTimeout, "time to wait for the connect mDNS announce (env: WADB_CONNECT_TIMEOUT)")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if flag.NArg() > 0 {
		if flag.Arg(0) == "doctor" && flag.NArg() == 1 {
			if err := doctor(*adbPath, *verbose); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintf(os.Stderr, "error: unexpected positional arguments: %v\n\n", flag.Args())
		flag.Usage()
		os.Exit(2)
	}

	opts := runOptions{
		ADBPath:        *adbPath,
		PairingTimeout: *pairingTimeout,
		ConnectTimeout: *connectTimeout,
		PairOnly:       *pairOnly,
		QRASCII:        *qrASCII,
		Verbose:        *verbose,
	}
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprintln(w, "wadb — pair Android devices over ADB Wi-Fi via a terminal QR code.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  wadb [flags]")
	fmt.Fprintln(w, "  wadb [flags] doctor")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "With no flags, wadb prints a QR code. Scan it from")
	fmt.Fprintln(w, "Settings → Developer options → Wireless debugging → Pair device with QR code")
	fmt.Fprintln(w, "on an Android 11+ device sharing the same Wi-Fi network. wadb will")
	fmt.Fprintln(w, "pair and connect automatically, then exit.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	flag.PrintDefaults()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintln(w, "  WADB_ADB, WADB_PAIR_ONLY, WADB_QR_ASCII, WADB_VERBOSE, WADB_PAIR_TIMEOUT, WADB_CONNECT_TIMEOUT")
	fmt.Fprintln(w, "  CLI flags override environment values.")
}

func doctor(adbPath string, verbose bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		if verbose {
			fmt.Println(adbVersion.Raw)
		}
	}

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

type runOptions struct {
	ADBPath        string
	PairingTimeout time.Duration
	ConnectTimeout time.Duration
	PairOnly       bool
	QRASCII        bool
	Verbose        bool
}

func loadEnvOptions() (runOptions, error) {
	opts := runOptions{
		PairingTimeout: defaultPairingTimeout,
		ConnectTimeout: defaultConnectTimeout,
	}

	opts.ADBPath = strings.TrimSpace(os.Getenv("WADB_ADB"))

	var err error
	if opts.PairOnly, err = envBool("WADB_PAIR_ONLY", opts.PairOnly); err != nil {
		return runOptions{}, err
	}
	if opts.QRASCII, err = envBool("WADB_QR_ASCII", opts.QRASCII); err != nil {
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

func run(opts runOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var logf mdns.Logf
	if opts.Verbose {
		logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	adbPath := opts.ADBPath
	if adbPath == "" {
		found, err := findADB()
		if err != nil {
			return err
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
	pairing.RenderQR(os.Stdout, payload, pairing.QROptions{ASCII: opts.QRASCII})
	fmt.Println()
	fmt.Println("Waiting for pairing announce...")

	pairCtx, cancelPair := context.WithTimeout(ctx, opts.PairingTimeout)
	defer cancelPair()
	pairEP, err := browsePairing(pairCtx, serviceName, logf)
	if err != nil {
		return fmt.Errorf("did not see _adb-tls-pairing._tcp announce within %s: %w\ncheck that both devices are on the same Wi-Fi, Wireless debugging is enabled, and mDNS/UDP 5353 is not blocked by the network or firewall", opts.PairingTimeout, err)
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
	connCtx, cancelConn := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancelConn()
	connEPs, err := browseConnect(connCtx, connectSettleDelay, logf)
	if err != nil {
		return fmt.Errorf("paired successfully, but no _adb-tls-connect._tcp announce appeared within %s: %w\nsome Android builds delay this announce; retry wadb, or run adb connect manually using the host and port shown in Wireless debugging", opts.ConnectTimeout, err)
	}

	var failures []string
	for _, connEP := range connEPs {
		fmt.Printf("Connecting to %s:%d...\n", connEP.Host, connEP.Port)
		out, err := adbConnect(ctx, adbPath, connEP.Host, connEP.Port)
		if err == nil {
			fmt.Println(out)
			return nil
		}
		failures = append(failures, err.Error())
	}

	return fmt.Errorf("failed to connect to %d discovered endpoint(s): %s", len(connEPs), strings.Join(failures, "; "))
}
