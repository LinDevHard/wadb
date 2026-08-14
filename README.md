# wadb

[![CI](https://github.com/LinDevHard/wadb/actions/workflows/ci.yml/badge.svg)](https://github.com/LinDevHard/wadb/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/LinDevHard/wadb)](https://github.com/LinDevHard/wadb/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/LinDevHard/wadb)](go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/lindevhard/wadb.svg)](https://pkg.go.dev/github.com/lindevhard/wadb)
[![Go Report Card](https://goreportcard.com/badge/github.com/lindevhard/wadb)](https://goreportcard.com/report/github.com/lindevhard/wadb)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Connect an Android 11+ device over ADB Wi-Fi by scanning a QR code from your terminal.

Same protocol as Android Studio's *Pair device using Wi-Fi*, but without launching the IDE — just run `wadb`, scan, done.

![demo](docs/demo.gif)

## Install

Requires Go (see [go.mod](go.mod) for the minimum version) and `adb` from Android platform-tools.

```sh
go install github.com/lindevhard/wadb@latest
```

Homebrew:

```sh
brew tap LinDevHard/tap
brew install wadb
```

Or build from a clone:

```sh
git clone https://github.com/LinDevHard/wadb && cd wadb
go build -o wadb .
```

## Usage

```sh
wadb
```

A QR code prints in the terminal. On your phone open
**Settings → Developer options → Wireless debugging → Pair device with QR code** and scan it. `wadb` will pair and connect automatically, then exit.

Both the phone and the host must be on the same Wi-Fi network (no AP isolation between clients), and Wireless debugging must be enabled in Developer options.

### Reconnecting

Pairing survives reboots and Wi-Fi reconnects, but the port Android listens on does not — which is why a previously paired device disappears from `adb devices` after a reboot. Once a device has been paired, bring it back without scanning anything:

```sh
wadb connect
```

This skips the QR flow entirely: it browses `_adb-tls-connect._tcp` and connects to the first discovered device this host is already paired with. Devices paired with someone else are discovered too, but they reject the connection, so `wadb connect` simply moves on to the next one. Raise `--connect-timeout` if your device is slow to announce.

Useful diagnostics:

```sh
wadb doctor
wadb --verbose
wadb --pair-timeout 3m --connect-timeout 45s
wadb --adb /path/to/adb
wadb --pair-only
wadb --qr-ascii
WADB_ADB=/path/to/adb WADB_PAIR_ONLY=true wadb
```

`doctor` checks the local `adb`, starts the server, and prints mDNS services reported by `adb mdns services`. `--verbose` prints discovered mDNS entries during pairing.

Use `--adb` when you want to force a specific Android platform-tools install. Use `--pair-only` when pairing works but your device delays or hides the `_adb-tls-connect._tcp` announce; after it exits, connect manually with the host and port shown in Android's Wireless debugging screen. Use `--qr-ascii` if your terminal font or emulator renders the default compact QR poorly.

The same options can be set with environment variables: `WADB_ADB`, `WADB_PAIR_ONLY`, `WADB_QR_ASCII`, `WADB_VERBOSE`, `WADB_PAIR_TIMEOUT`, and `WADB_CONNECT_TIMEOUT`. CLI flags override environment values. Boolean variables accept values like `true`, `false`, `1`, or `0`; timeout variables use durations like `30s` or `3m`.

## How it works

1. `wadb` locates the local `adb` binary.
2. It generates a single-use service name (`studio-<random>`) and password, then renders them as a QR code with payload `WIFI:T:ADB;S:...;P:...;;` — the same format Android Studio uses.
3. When the phone scans the QR, it advertises `_adb-tls-pairing._tcp` via mDNS. `wadb` matches the announce by instance name and runs `adb pair`.
4. After pairing succeeds, the phone advertises `_adb-tls-connect._tcp`. `wadb` tries connect endpoints from the same IP as the pairing announce first, then falls back to other discovered endpoints.
5. After a successful `adb connect`, `wadb` prints the result and, when available, the device name from Android system properties.

The actual TLS pairing handshake is handled by `adb pair`; `wadb` only orchestrates discovery and credential generation.

## Background

For a deeper explanation of the ADB Wi-Fi pairing flow, mDNS discovery, and newer reconnect improvements, see the Android Makers/droidCon 2026 talk [How ADB Wifi 2.0 works](https://www.youtube.com/watch?v=_CR44gRhad4).

## `adb` discovery

`wadb` searches for `adb` in this order and uses the first match:

1. `$ANDROID_HOME/platform-tools/adb`
2. `$ANDROID_SDK_ROOT/platform-tools/adb`
3. `~/Library/Android/sdk/platform-tools/adb` (macOS Android Studio default)
4. `~/Android/Sdk/platform-tools/adb` (Linux default)
5. `adb` on `$PATH` (e.g. Homebrew `android-platform-tools`)
6. `/opt/homebrew/share/android-commandlinetools/platform-tools/adb`

If none match, set `ANDROID_HOME` or install platform-tools.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| *"did not see device announce within 2m"* | Phone could not reach the host over mDNS. Check same Wi-Fi subnet, no AP isolation, firewall not blocking UDP 5353. |
| `adb pair` fails immediately | Stale daemon. Run `adb kill-server` and retry. |
| Connect timeout after a successful pair | Some Android builds delay the connect announce. Run `wadb connect` (no need to pair again), or run `adb connect <ip>:<port>` manually once Wireless debugging shows the device's port. |
| `wadb connect` finds nothing | The device is not advertising. Open Wireless debugging on the phone to wake the announce, and confirm the device was paired with this host. |

## Platform support

| Platform | Status |
| --- | --- |
| macOS | Supported and tested. |
| Linux | Supported, expected to work as-is. |
| Windows | Experimental. `adb` discovery is partially implemented and unverified; use `--adb` or `WADB_ADB` to point at a known platform-tools install. |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
