# Homebrew formula

This directory contains the formula intended for a future Homebrew tap, for example `LinDevHard/homebrew-tap`.

`wadb.rb` is a checked-in example for the current stable tag. `wadb.rb.template` is used by the release workflow to generate the tap formula for the tag being released.

To publish it:

```sh
mkdir -p Formula
cp packaging/homebrew/wadb.rb Formula/wadb.rb
```

For each release, update:

- `tag`
- `revision`

The formula builds from the tagged source with Go and depends on Homebrew's `android-platform-tools` package for `adb`.

The main release workflow uses GoReleaser for archives, checksums, and GitHub Releases, then updates `LinDevHard/homebrew-tap` over SSH. The source repository must have an Actions secret named `HOMEBREW_TAP_DEPLOY_KEY` containing a private deploy key whose public key is installed on the tap repository with write access.

GoReleaser can also publish Homebrew entries, but its formula publisher is deprecated in v2.10 in favor of casks. The dedicated tap-update step keeps `wadb` as a normal Homebrew formula without relying on deprecated GoReleaser config.
