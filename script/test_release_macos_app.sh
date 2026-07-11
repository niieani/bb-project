#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

"$ROOT_DIR/script/render_homebrew_menubar_cask.sh" \
  --version 1.2.3 \
  --sha256 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --repository niieani/bb-project \
  --output "$TEMP_DIR/bb-menubar.rb"

ruby -c "$TEMP_DIR/bb-menubar.rb" >/dev/null
grep -F 'version "1.2.3"' "$TEMP_DIR/bb-menubar.rb" >/dev/null
grep -F 'app "BBMenuBar.app"' "$TEMP_DIR/bb-menubar.rb" >/dev/null
grep -F 'depends_on cask: "bb"' "$TEMP_DIR/bb-menubar.rb" >/dev/null
grep -F 'releases/download/v#{version}/BBMenuBar_#{version}_macOS.zip' "$TEMP_DIR/bb-menubar.rb" >/dev/null

workflow="$ROOT_DIR/.github/workflows/release-publish.yml"
grep -F 'script/package_macos_app.sh' "$workflow" >/dev/null
grep -F 'runs-on: macos-26' "$workflow" >/dev/null
grep -F 'run: swift --version' "$workflow" >/dev/null
grep -F 'swift test --package-path "$PACKAGE_DIR"' "$ROOT_DIR/script/package_macos_app.sh" >/dev/null
grep -F 'op://Automation/Apple Developer App Store Connect AuthKey Github Releases/AuthKey.p8' "$workflow" >/dev/null
grep -F 'op://Automation/Apple Release Signing Developer ID Certificate/Apple Release Signing Developer ID Certificate.p12' "$workflow" >/dev/null
grep -F 'op://Automation/GitHub Token for homebrew-tap/token' "$workflow" >/dev/null
grep -F 'script/render_homebrew_menubar_cask.sh' "$workflow" >/dev/null
grep -F 'checksum="$archive.sha256"' "$workflow" >/dev/null
if grep -F 'secrets.HOMEBREW_TAP_GITHUB_TOKEN' "$workflow" >/dev/null; then
  echo "Homebrew token must be loaded from 1Password" >&2
  exit 1
fi

echo "release macOS app configuration: ok"
