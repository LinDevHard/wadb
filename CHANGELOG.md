# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### Added

- Added `wadb connect` to reconnect a device that is already paired with this host, without generating a QR code.
- Added `--iface` (env: `WADB_IFACE`) to browse mDNS on a single network interface when a VPN, container bridge, or second NIC swallows the multicast traffic.
- `wadb doctor` now lists the network interfaces worth passing to `--iface`, and validates one when given.

### Changed

- Fall back to the endpoints reported by `adb mdns services` when the built-in mDNS browse finds no `_adb-tls-connect._tcp` announce.

## [1.0.0] - 2026-06-02

### Added

- Added `--adb` to force a specific `adb` binary.
- Added `--pair-only` to stop after successful pairing without running `adb connect`.
- Added `--qr-ascii` for terminals that render the default compact QR poorly.
- Added environment variable equivalents for UX flags and timeouts.
- Prefer `_adb-tls-connect._tcp` endpoints from the same host as the pairing announce.
- Print the connected device name when Android system properties are available.
- Added SHA-256 checksum generation for release archives.
- Added a Homebrew formula for future tap publishing.
- Added GoReleaser configuration for release archives, checksums, and GitHub Releases.
- Added release workflow automation to update the Homebrew tap over SSH.

### Changed

- Marked Windows support as experimental until `adb` discovery is verified.
- Switched the release workflow from hand-rolled archive jobs to GoReleaser.

## [0.2.0] - 2026-05-25

### Added

- Added `--pair-timeout` and `--connect-timeout` flags.
- Added `--verbose` mDNS discovery logging.
- Added `wadb doctor` for local `adb` and mDNS diagnostics.
- Added a platform-tools version warning for older `adb` installations.
- Documented diagnostics options and linked the Android Makers/droidCon 2026 ADB Wi-Fi 2 talk.

### Changed

- Try multiple discovered `_adb-tls-connect._tcp` endpoints after pairing instead of only the first one.
- Improved pairing and connect timeout errors with actionable network and mDNS hints.
- Improved `wadb doctor` output when `adb mdns services` reports no discovered services.
- Added the demo GIF to the README.

## [0.1.1] - 2026-05-25

### Added

- Added `--version` and `--help` flags.
- Added cross-platform release builds in CI.
- Added SECURITY, CONTRIBUTING, and Code of Conduct documents.

### Changed

- Polished README metadata and project badges.

## [0.1.0] - 2026-05-25

### Added

- Initial release of `wadb`.
- Terminal QR code generation for Android 11+ ADB Wi-Fi pairing.
- mDNS discovery for `_adb-tls-pairing._tcp` and `_adb-tls-connect._tcp`.
- `adb pair` and `adb connect` orchestration.

[Unreleased]: https://github.com/LinDevHard/wadb/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/LinDevHard/wadb/compare/v0.2.0...v1.0.0
[0.2.0]: https://github.com/LinDevHard/wadb/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/LinDevHard/wadb/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/LinDevHard/wadb/releases/tag/v0.1.0
