# Homebrew formula

This directory contains the formula support for the `LinDevHard/homebrew-tap` repository.

`wadb.rb` is a checked-in example for the current stable tag. `wadb.rb.template` is used by the release workflow to generate the tap formula for the tag being released.

To publish it manually:

```sh
mkdir -p Formula
cp packaging/homebrew/wadb.rb Formula/wadb.rb
```

For each release, update:

- `version`
- release asset URLs
- release asset `sha256` values

The formula installs prebuilt release archives and prints caveats for installing Android platform-tools when `adb` is not already available.

The main release workflow uses GoReleaser for archives, checksums, and GitHub Releases, then updates `LinDevHard/homebrew-tap` over SSH from the generated `dist/checksums.txt`. The source repository must have an Actions secret named `HOMEBREW_TAP_DEPLOY_KEY` containing a private deploy key whose public key is installed on the tap repository with write access.

GoReleaser can also publish Homebrew entries, but its formula publisher is deprecated in v2.10 in favor of casks. The dedicated tap-update step keeps `wadb` as a normal Homebrew formula without relying on deprecated GoReleaser config.
