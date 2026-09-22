package main

import (
	"context"
	"errors"
	"fmt"
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
)

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
	_, err := tryConnectEndpoints(ctx, adbPath, endpoints, false, false)
	return err
}

type connectionResult struct {
	Address  string `json:"address"`
	Instance string `json:"instance,omitempty"`
	Status   string `json:"status"`
	Device   string `json:"device,omitempty"`
	Error    string `json:"error,omitempty"`
}

func connectToEndpointsMode(ctx context.Context, adbPath string, endpoints []mdns.Endpoint, all, jsonOutput bool) error {
	results, err := tryConnectEndpoints(ctx, adbPath, endpoints, all, jsonOutput)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(results)
	}
	return nil
}

func tryConnectEndpoints(ctx context.Context, adbPath string, endpoints []mdns.Endpoint, all, quiet bool) ([]connectionResult, error) {
	var failures []string
	var results []connectionResult
	succeeded := 0
	for _, ep := range endpoints {
		addr := net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port))
		if !quiet {
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
			if !quiet {
				fmt.Println(out)
				if result.Device != "" {
					fmt.Println("Device:", result.Device)
				}
			}
			if !all {
				return results, nil
			}
			continue
		}
		failures = append(failures, err.Error())
		results = append(results, connectionResult{Address: addr, Instance: ep.Instance, Status: "failed", Error: err.Error()})
		if all && !quiet {
			fmt.Fprintln(os.Stderr, "Warning:", err)
		}
	}
	if succeeded > 0 {
		return results, nil
	}
	return results, fmt.Errorf("failed to connect to %d discovered endpoint(s): %s", len(endpoints), strings.Join(failures, "; "))
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
			results, connectErr := tryConnectEndpoints(ctx, adbPath, []mdns.Endpoint{endpoint}, false, opts.machineOutput())
			if connectErr != nil {
				return withErrorCode("connection_failed", connectErr, "Confirm that this host is paired with the device and retry while Wireless debugging is open.")
			}
			if opts.structuredJSON() {
				return writeJSONSuccess("connect", map[string]any{"connections": results})
			}
			if opts.JSON {
				return writeJSON(results)
			}
			return nil
		}
	}

	if !opts.machineOutput() {
		fmt.Println("Looking for devices announcing _adb-tls-connect._tcp...")
	}
	endpoints, err := discoverConnectEndpoints(ctx, adbPath, opts.ConnectTimeout, "", mdnsOpts)
	if err != nil {
		wrapped := fmt.Errorf("no _adb-tls-connect._tcp announce appeared within %s: %w\nenable Wireless debugging on a device already paired with this host, or run wadb without arguments to pair a new one\nif a VPN or container bridge is active, limit discovery with --iface (wadb doctor lists the candidates)", opts.ConnectTimeout, err)
		return withErrorCode("discovery_timeout", wrapped, "Enable Wireless debugging, then retry; use wadb doctor --output json if discovery still fails.")
	}

	endpoints = selectEndpoints(endpoints, opts.Device)
	if len(endpoints) == 0 {
		return withErrorCode("device_not_found", fmt.Errorf("no discovered device matches %q", opts.Device), "Run wadb devices --output json and select an id or address from the result.")
	}
	if opts.structuredJSON() && opts.Device == "" && !opts.All && len(endpoints) > 1 {
		return withErrorCode("multiple_devices", fmt.Errorf("found %d wireless devices; refusing to choose one in structured JSON mode", len(endpoints)), "Run wadb devices --output json, then pass --device <id|address>, or explicitly pass --all.")
	}
	results, err := tryConnectEndpoints(ctx, adbPath, endpoints, opts.All, opts.machineOutput())
	if err != nil {
		return withErrorCode("connection_failed", err, "Confirm that this host is paired with the device and retry while Wireless debugging is open.")
	}
	if opts.structuredJSON() {
		return writeJSONSuccess("connect", map[string]any{"connections": results})
	}
	if opts.JSON {
		return writeJSON(results)
	}
	return nil
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
		return withErrorCode("adb_error", err, "Check the adb server and run wadb doctor --output json for diagnostics.")
	}
	announced, scanErr := scanConnectEndpoints(ctx, adbPath, opts.ScanTimeout, mdnsOpts)
	if scanErr != nil && opts.Verbose {
		fmt.Fprintln(os.Stderr, "Warning: device scan:", scanErr)
	}
	rows := mergeManagedDevices(connected, announced)
	rows = selectManagedDevices(rows, opts.Device)
	if opts.structuredJSON() {
		return writeJSONSuccess("devices", map[string]any{"devices": rows})
	}
	if opts.JSON {
		return writeJSON(rows)
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
		if err != nil {
			return withErrorCode("disconnect_failed", err, "Run wadb devices --output json and retry with an explicit device address.")
		}
		if opts.structuredJSON() {
			return writeJSONSuccess("disconnect", map[string]any{"disconnections": []disconnectResult{result}})
		} else if opts.JSON {
			return writeJSON([]disconnectResult{result})
		} else if out != "" {
			fmt.Println(out)
		}
		return nil
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
		} else if !opts.machineOutput() && out != "" {
			fmt.Println(out)
		}
		results = append(results, result)
	}
	if len(failures) > 0 {
		return withErrorCode("disconnect_failed", errors.New(strings.Join(failures, "; ")), "Run wadb devices --output json and retry with the current wireless address.")
	}
	if opts.structuredJSON() {
		return writeJSONSuccess("disconnect", map[string]any{"disconnections": results})
	}
	if opts.JSON {
		return writeJSON(results)
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
			wrapped := fmt.Errorf("no wireless device matches %q; discovery failed: %w", opts.Device, scanErr)
			return nil, withErrorCode("device_not_found", wrapped, "Run wadb devices --output json and select a current wireless address.")
		}
		return nil, withErrorCode("device_not_found", fmt.Errorf("no wireless device matches %q", opts.Device), "Run wadb devices --output json and select a current wireless address.")
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
