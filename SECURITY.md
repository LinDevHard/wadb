# Security Policy

## Supported Versions

This project follows the latest tagged release on `main`. Older releases do not receive backports.

## Reporting a Vulnerability

Please report suspected vulnerabilities **privately** by email to **lindevhard@gmail.com** rather than opening a public issue or pull request.

When reporting, include:
- A clear description of the issue and its impact.
- Steps to reproduce, ideally a minimal proof-of-concept.
- The version of `wadb` (`wadb --version`) and your OS / `adb` version.

You can expect an initial response within a few days. I will work with you on a fix and coordinate disclosure once a patched release is available.

## Scope

`wadb` orchestrates Android Studio's wireless pairing flow over local mDNS and shells out to `adb`. Reports about issues in the underlying ADB protocol, the Android device, or third-party dependencies should be filed with their respective projects; I am happy to help triage if you are unsure where a report belongs.
