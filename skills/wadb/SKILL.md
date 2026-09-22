---
name: wadb
description: Pair, reconnect, inspect, diagnose, or disconnect Android 11+ devices over ADB Wi-Fi with the wadb CLI. Use for Android Wireless debugging, adb pair, adb connect, QR or six-digit-code pairing, and devices that disappear from adb after reboot. Requires local shell access; do not use for USB-only ADB work.
---

# wadb

Use `wadb` for Android Wireless debugging on the user's local network. It orchestrates the installed `adb`; the computer and Android device must be on the same Wi-Fi network, and the device must have Wireless debugging enabled.

## Choose the operation

- Diagnose setup or discovery with `wadb doctor --non-interactive --output json`.
- Inspect devices with `wadb devices --non-interactive --output json` before selecting or changing a connection.
- Reconnect an already paired device with `wadb connect --device <id-or-address> --non-interactive --output json`.
- Pair from Android's six-digit-code dialog with `wadb pair <host:port> --non-interactive --output json`. Start the process, then provide the code through redirected stdin. Never put the pairing code in command arguments, environment variables, shell history, logs, or the response to the user.
- Use plain `wadb` for QR pairing. Tell the user to scan the displayed QR from **Settings -> Developer options -> Wireless debugging -> Pair device with QR code**. QR pairing is interactive and does not support structured output.
- Disconnect only when the user requests it. Use `wadb disconnect --device <id-or-address> --non-interactive --output json` for one device. Use `--all` only when the user explicitly asks to affect every wireless device.

If `connect --output json` reports `multiple_devices`, use the IDs from `devices --output json`; do not choose for the user. If discovery fails, keep Wireless debugging open on the phone and use the suggestion returned in the JSON error before escalating diagnostics.

## Interpret results

With `--output json`, stdout contains one JSON object. Read `ok`, then inspect `result` or `error.code`. Use `error.retryable` to decide whether another attempt is reasonable and stop for the user when `error.requires_user_action` is true. Progress and diagnostics may appear on stderr. Schema version `1` has this top-level shape:

```json
{
  "schema_version": 1,
  "ok": true,
  "action": "devices",
  "result": {}
}
```

Exit status `0` means success, `1` means the operation failed, and `2` means the request or environment options were invalid. A nonzero exit status with `--output json` still returns the structured error on stdout.

If `wadb` is unavailable, report that it and Android platform-tools are required. Follow the host's normal authorization process before installing software.
