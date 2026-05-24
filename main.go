package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
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

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "shorthand for --version")
	verbose := flag.Bool("verbose", false, "print discovered mDNS service entries to stderr")
	pairingTimeout := flag.Duration("pair-timeout", defaultPairingTimeout, "time to wait for the pairing mDNS announce")
	connectTimeout := flag.Duration("connect-timeout", defaultConnectTimeout, "time to wait for the connect mDNS announce")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected positional arguments: %v\n\n", flag.Args())
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*pairingTimeout, *connectTimeout, *verbose); err != nil {
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
	fmt.Fprintln(w)
	fmt.Fprintln(w, "With no flags, wadb prints a QR code. Scan it from")
	fmt.Fprintln(w, "Settings → Developer options → Wireless debugging → Pair device with QR code")
	fmt.Fprintln(w, "on an Android 11+ device sharing the same Wi-Fi network. wadb will")
	fmt.Fprintln(w, "pair and connect automatically, then exit.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	flag.PrintDefaults()
}

func run(pairingTimeout, connectTimeout time.Duration, verbose bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var logf mdns.Logf
	if verbose {
		logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	adbPath, err := adb.Find()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Using adb:", adbPath)

	adbVersion, err := adb.Version(ctx, adbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	} else if adbVersion.PlatformToolsMajor > 0 {
		fmt.Fprintf(os.Stderr, "Using platform-tools: %d\n", adbVersion.PlatformToolsMajor)
		if !adbVersion.SupportsWifi2Improvements() {
			fmt.Fprintf(os.Stderr, "Warning: platform-tools before %d may miss newer ADB Wi-Fi mDNS and reconnect improvements.\n", adb.Wifi2PlatformToolsMajor)
		}
	} else if verbose {
		fmt.Fprintln(os.Stderr, "adb version output:")
		fmt.Fprintln(os.Stderr, adbVersion.Raw)
	}

	if err := adb.StartServer(ctx, adbPath); err != nil {
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
	pairing.RenderQR(os.Stdout, payload)
	fmt.Println()
	fmt.Println("Waiting for pairing announce...")

	pairCtx, cancelPair := context.WithTimeout(ctx, pairingTimeout)
	defer cancelPair()
	pairEP, err := mdns.BrowsePairing(pairCtx, serviceName, logf)
	if err != nil {
		return fmt.Errorf("did not see _adb-tls-pairing._tcp announce within %s: %w\ncheck that both devices are on the same Wi-Fi, Wireless debugging is enabled, and mDNS/UDP 5353 is not blocked by the network or firewall", pairingTimeout, err)
	}
	fmt.Printf("Found pairing endpoint %s:%d, pairing...\n", pairEP.Host, pairEP.Port)

	if err := adb.Pair(ctx, adbPath, pairEP.Host, pairEP.Port, password); err != nil {
		return err
	}
	fmt.Println("Paired successfully.")

	fmt.Println("Waiting for device to announce on _adb-tls-connect._tcp...")
	connCtx, cancelConn := context.WithTimeout(ctx, connectTimeout)
	defer cancelConn()
	connEPs, err := mdns.BrowseConnect(connCtx, connectSettleDelay, logf)
	if err != nil {
		return fmt.Errorf("paired successfully, but no _adb-tls-connect._tcp announce appeared within %s: %w\nsome Android builds delay this announce; retry wadb, or run adb connect manually using the host and port shown in Wireless debugging", connectTimeout, err)
	}

	var failures []string
	for _, connEP := range connEPs {
		fmt.Printf("Connecting to %s:%d...\n", connEP.Host, connEP.Port)
		out, err := adb.Connect(ctx, adbPath, connEP.Host, connEP.Port)
		if err == nil {
			fmt.Println(out)
			return nil
		}
		failures = append(failures, err.Error())
	}

	return fmt.Errorf("failed to connect to %d discovered endpoint(s): %s", len(connEPs), strings.Join(failures, "; "))
}
