# wadb

CLI for pairing and connecting an Android 11+ device over ADB Wi-Fi by scanning a QR code from the terminal. Same protocol as Android Studio's "Pair device using Wi-Fi", without launching Android Studio.

## How it works

1. `wadb` finds your local `adb` binary.
2. It generates a one-shot service name and password, renders them as a QR code in the terminal (`WIFI:T:ADB;S:studio-...;P:...;;`).
3. You open **Settings → Developer options → Wireless debugging → Pair device with QR code** on the phone and scan.
4. The phone announces `_adb-tls-pairing._tcp` via mDNS. `wadb` matches the announce by instance name and runs `adb pair`.
5. After pairing, the phone announces `_adb-tls-connect._tcp`. `wadb` runs `adb connect` and prints the result.

## Install

Requires Go 1.21+ and `adb` from Android platform-tools.

```sh
go install github.com/lindevhard/wadb@latest
```

Or build from this repo:

```sh
go build -o wadb .
```

## Usage

```sh
wadb
```

The phone and host must be on the same Wi-Fi network. Wireless debugging must be enabled on the phone (Developer options).

## adb discovery

`wadb` looks for `adb` in this order:

1. `$ANDROID_HOME/platform-tools/adb`
2. `$ANDROID_SDK_ROOT/platform-tools/adb`
3. `~/Library/Android/sdk/platform-tools/adb` (macOS Android Studio default)
4. `~/Android/Sdk/platform-tools/adb` (Linux default)
5. `adb` on `$PATH` (Homebrew `android-platform-tools`)
6. `/opt/homebrew/share/android-commandlinetools/platform-tools/adb`

If none match, set `ANDROID_HOME` or install platform-tools.

## Troubleshooting

- **"did not see device announce within 2m"** — phone could not reach the host over mDNS. Confirm same Wi-Fi, no AP isolation, IPv4/IPv6 not blocked between clients.
- **`adb pair` fails immediately** — most often a stale daemon. `adb kill-server` and retry.
- **Connect timeout after successful pair** — some Android builds delay the connect-service announce. Re-run `wadb` or use `adb connect <ip>:<port>` manually once Wireless debugging shows the port.

## Status

MVP. Platform tested: macOS. Linux should work as-is. Windows adb discovery is stubbed but not exercised.
