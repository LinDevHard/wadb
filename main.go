package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lindevhard/wadb/internal/adb"
	"github.com/lindevhard/wadb/internal/mdns"
	"github.com/lindevhard/wadb/internal/pairing"
)

const (
	pairingTimeout = 120 * time.Second
	connectTimeout = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	adbPath, err := adb.Find()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Using adb:", adbPath)

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
	pairEP, err := mdns.BrowsePairing(pairCtx, serviceName)
	if err != nil {
		return fmt.Errorf("did not see device announce within %s: %w", pairingTimeout, err)
	}
	fmt.Printf("Found pairing endpoint %s:%d, pairing...\n", pairEP.Host, pairEP.Port)

	if err := adb.Pair(ctx, adbPath, pairEP.Host, pairEP.Port, password); err != nil {
		return err
	}
	fmt.Println("Paired successfully.")

	fmt.Println("Waiting for device to announce on _adb-tls-connect._tcp...")
	connCtx, cancelConn := context.WithTimeout(ctx, connectTimeout)
	defer cancelConn()
	connEP, err := mdns.BrowseConnect(connCtx)
	if err != nil {
		return fmt.Errorf("device did not announce connect service: %w", err)
	}

	out, err := adb.Connect(ctx, adbPath, connEP.Host, connEP.Port)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
