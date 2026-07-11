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

The app requires macOS 14 or later. Its cask depends on the tap's `bb` cask, so a first install provides the CLI that the app invokes; future upgrades remain independent and explicit. On first launch it requests notification permission and registers with macOS launch-at-login; verify both against the installed signed bundle, not a SwiftPM development executable.

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

The release job bounds its synchronous notarization wait at 30 minutes. Apple continues processing a timed-out submission, while the job fails promptly instead of occupying the hosted runner for GitHub's six-hour maximum. Retry the tagged recovery workflow after the Notary service recovers; do not publish an unstapled app.

Swift tests, the universal build, and timestamped codesigning also have explicit stage names and 10/15/5-minute bounds. A manual tagged recovery checks out the immutable release source, then overlays only the current packaging harness from `main`; release recovery fixes therefore do not require moving an existing tag.

Then verify the menu reports no notification or launch-at-login error, trigger an eligible attention snapshot, confirm the banner is attributed to BBMenuBar, click it to open the exact digest list, and confirm interval/wake refresh can deliver without a terminal process.

Local unsigned packaging/config checks:

```bash
script/test_release_macos_app.sh
script/package_macos_app.sh --version 0.0.0 --output-dir temp.local/$(date +%F)/menubar-release
```

Release-path concurrency tests use child-process handshakes, not wall-clock deadlines. Hosted macOS runner load can delay task scheduling without blocking the MainActor; fixed millisecond thresholds are therefore invalid release gates.
