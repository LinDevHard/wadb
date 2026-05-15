# Contributing

Thanks for your interest in `wadb`. This is a small project — keep changes focused and the bar for review will stay low.

## Quick start

```sh
git clone https://github.com/LinDevHard/wadb
cd wadb
go test ./...
go build .
```

You need Go (see [go.mod](go.mod) for the minimum version) and `adb` from Android platform-tools for end-to-end testing.

## Before opening a PR

- `go vet ./...` — clean
- `go test -race ./...` — passes
- Run the binary against a real Android device if your change touches mDNS, pairing, or `adb` invocation. Unit tests cover the pure-logic parts; the wireless flow is not exercised in CI.
- Keep commits small and the message focused on **why**, not just **what**.

## Scope

Welcome:
- Bug fixes (especially around mDNS edge cases on different OS / network configs).
- Better `adb` discovery on Windows (the path is currently stubbed).
- Smaller, sharper error messages.
- Tests for behaviour that's currently uncovered.

Out of scope for now:
- Daemon / always-on mode.
- GUI or TUI front-ends.
- Reimplementing the ADB pairing TLS handshake (we shell out to `adb pair` on purpose).

If unsure, open an issue first and ask before writing code.

## Reporting bugs

Include:
- OS + version, Go version (`go version`), `adb --version`.
- Android version and device model.
- Exact command, full stdout/stderr, what you expected vs what happened.
- Whether the phone and host are on the same Wi-Fi subnet (no AP isolation).

## Code style

Standard Go. `gofmt -s` (run automatically by most editors). Prefer the standard library; reach for a dependency only when it pulls real weight.

## Code of conduct

By participating you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).
