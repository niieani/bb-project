# Releasing the macOS menu bar app

`BBMenuBar.app` ships from the same tagged-release workflow as `bb`. The reusable publish workflow has two sequential jobs:

1. GoReleaser publishes signed/notarized `bb` archives and updates the `bb` Homebrew cask.
2. A macOS runner tests and release-builds the Swift package, imports the existing Developer ID certificate from 1Password, signs with hardened runtime, notarizes and staples the app, uploads `BBMenuBar_<version>_macOS.zip`, then updates `Casks/bb-menubar.rb` in `niieani/homebrew-tap`.

Both jobs use the single `OP_SERVICE_ACCOUNT_TOKEN` GitHub secret. Apple credentials and the Homebrew token remain in the `Automation` 1Password vault; there is no app-specific signing configuration or GitHub secret.

## Install and upgrade

```bash
brew install --cask niieani/tap/bb-menubar
brew upgrade --cask bb-menubar
open -a BBMenuBar
```

The app requires macOS 14 or later. Its cask depends on the tap's `bb` cask, so a first install provides the CLI that the app invokes; future upgrades remain independent and explicit.

## Release verification

Download the app ZIP from the GitHub release, expand it, then verify the exact distributed bundle:

```bash
shasum -a 256 -c BBMenuBar_<version>_macOS.zip.sha256
ditto -x -k BBMenuBar_<version>_macOS.zip ./expanded
codesign --verify --deep --strict --verbose=2 ./expanded/BBMenuBar.app
spctl --assess --type execute --verbose=4 ./expanded/BBMenuBar.app
xcrun stapler validate ./expanded/BBMenuBar.app
```

Expected Gatekeeper assessment: accepted, source `Notarized Developer ID`. Also inspect the workflow's `notarytool` result for `status: Accepted`, verify the release asset checksum, and run:

```bash
brew audit --cask --strict niieani/tap/bb-menubar
brew reinstall --cask niieani/tap/bb-menubar
open -a BBMenuBar
```

Local unsigned packaging/config checks:

```bash
script/test_release_macos_app.sh
script/package_macos_app.sh --version 0.0.0 --output-dir temp.local/$(date +%F)/menubar-release
```
