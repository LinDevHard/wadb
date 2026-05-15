# wadb

[![CI](https://github.com/LinDevHard/wadb/actions/workflows/ci.yml/badge.svg)](https://github.com/LinDevHard/wadb/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/lindevhard/wadb.svg)](https://pkg.go.dev/github.com/lindevhard/wadb)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Connect an Android 11+ device over ADB Wi-Fi by scanning a QR code from your terminal.

Same protocol as Android Studio's *Pair device using Wi-Fi*, but without launching the IDE — just run `wadb`, scan, done.

## Install

Requires Go (see [go.mod](go.mod) for the minimum version) and `adb` from Android platform-tools.

```sh
go install github.com/lindevhard/wadb@latest
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

## How it works

1. `wadb` locates the local `adb` binary.
2. It generates a single-use service name (`studio-<random>`) and password, then renders them as a QR code with payload `WIFI:T:ADB;S:...;P:...;;` — the same format Android Studio uses.
3. When the phone scans the QR, it advertises `_adb-tls-pairing._tcp` via mDNS. `wadb` matches the announce by instance name and runs `adb pair`.
4. After pairing succeeds, the phone advertises `_adb-tls-connect._tcp`. `wadb` runs `adb connect` against it and prints the result.

The actual TLS pairing handshake is handled by `adb pair`; `wadb` only orchestrates discovery and credential generation.

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
| Connect timeout after a successful pair | Some Android builds delay the connect announce. Re-run `wadb`, or run `adb connect <ip>:<port>` manually once Wireless debugging shows the device's port. |

## Platform support

Tested on macOS. Linux should work as-is. Windows `adb` discovery is partially implemented but unverified — PRs welcome.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)

## Acknowledgments

- The ADB wireless pairing protocol is documented in the [LineageOS adb_wifi.md](https://github.com/LineageOS/android_packages_modules_adb/blob/lineage-23.2/docs/dev/adb_wifi.md) (fork of AOSP).
- Reference implementation in Python: [Vazgen005/adb-wifi-py](https://github.com/Vazgen005/adb-wifi-py).
