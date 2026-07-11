#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="$ROOT_DIR/macos/BBMenuBar"
APP_NAME="BBMenuBar"
BUNDLE_ID="dev.niieani.bb-menubar"
MIN_SYSTEM_VERSION="14.0"
NOTARY_WAIT_TIMEOUT="30m"

run_with_timeout() {
  local seconds="${1:?missing timeout seconds}"
  shift
  /usr/bin/perl -e 'alarm shift; exec @ARGV' "$seconds" "$@"
}

version=""
output_dir=""
stage="all"
sign_identity="${MACOS_SIGN_IDENTITY:-}"
notary_key="${MACOS_NOTARY_KEY_PATH:-}"
notary_key_id="${MACOS_NOTARY_KEY_ID:-}"
notary_issuer_id="${MACOS_NOTARY_ISSUER_ID:-}"

while (($#)); do
  case "$1" in
    --version) version="${2:?missing version}"; shift 2 ;;
    --output-dir) output_dir="${2:?missing output directory}"; shift 2 ;;
    --stage) stage="${2:?missing stage}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$version" || -z "$output_dir" ]]; then
  echo "usage: $0 --version VERSION --output-dir PATH" >&2
  exit 2
fi
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid version: $version" >&2
  exit 2
fi
case "$stage" in
  all|test|build|sign|notarize|archive) ;;
  *) echo "invalid stage: $stage" >&2; exit 2 ;;
esac

mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
app_bundle="$output_dir/$APP_NAME.app"
app_contents="$app_bundle/Contents"
app_macos="$app_contents/MacOS"
archive="$output_dir/${APP_NAME}_${version}_macOS.zip"

run_tests() {
  echo "release: running Swift tests (10m timeout)" >&2
  LIBDISPATCH_COOPERATIVE_POOL_STRICT=1 \
    run_with_timeout 600 swift test --package-path "$PACKAGE_DIR" >&2
}

build_bundle() {
  echo "release: building universal app (15m timeout)" >&2
  run_with_timeout 900 swift build --configuration release --package-path "$PACKAGE_DIR" --arch arm64 --arch x86_64 >&2
  local binary
  binary="$(swift build --configuration release --package-path "$PACKAGE_DIR" --arch arm64 --arch x86_64 --show-bin-path)/$APP_NAME"
  rm -rf "$app_bundle" "$archive"
  mkdir -p "$app_macos"
  install -m 0755 "$binary" "$app_macos/$APP_NAME"
  cat >"$app_contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>$APP_NAME</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleName</key><string>$APP_NAME</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleVersion</key><string>$version</string>
  <key>LSMinimumSystemVersion</key><string>$MIN_SYSTEM_VERSION</string>
  <key>LSUIElement</key><true/>
  <key>NSPrincipalClass</key><string>NSApplication</string>
</dict>
</plist>
PLIST
  plutil -lint "$app_contents/Info.plist" >&2
}

signing_requested=0
[[ -n "$sign_identity" || -n "$notary_key" || -n "$notary_key_id" || -n "$notary_issuer_id" ]] && signing_requested=1
require_signing_credentials() {
  if [[ -z "$sign_identity" || -z "$notary_key" || -z "$notary_key_id" || -z "$notary_issuer_id" ]]; then
    echo "signing requires MACOS_SIGN_IDENTITY, MACOS_NOTARY_KEY_PATH, MACOS_NOTARY_KEY_ID, and MACOS_NOTARY_ISSUER_ID" >&2
    exit 1
  fi
}

sign_bundle() {
  require_signing_credentials
  echo "release: signing app with trusted timestamp (5m timeout)" >&2
  run_with_timeout 300 codesign --force --options runtime --timestamp --sign "$sign_identity" "$app_bundle"
  codesign --verify --deep --strict --verbose=2 "$app_bundle" >&2
}

notarize_bundle() {
  require_signing_credentials
  ditto --norsrc --noextattr -c -k --keepParent "$app_bundle" "$archive"
  echo "release: submitting app for notarization ($NOTARY_WAIT_TIMEOUT timeout)" >&2
  xcrun notarytool submit "$archive" --key "$notary_key" --key-id "$notary_key_id" --issuer "$notary_issuer_id" --wait --timeout "$NOTARY_WAIT_TIMEOUT" >&2
  xcrun stapler staple "$app_bundle" >&2
  xcrun stapler validate "$app_bundle" >&2
  rm -f "$archive"
}

archive_bundle() {
  rm -f "$archive"
  ditto --norsrc --noextattr -c -k --keepParent "$app_bundle" "$archive"
  printf '%s\n' "$archive"
}

case "$stage" in
  test) run_tests ;;
  build) build_bundle ;;
  sign) sign_bundle ;;
  notarize) notarize_bundle ;;
  archive) archive_bundle ;;
  all)
    run_tests
    build_bundle
    if ((signing_requested)); then
      sign_bundle
      notarize_bundle
    fi
    archive_bundle
    ;;
esac
