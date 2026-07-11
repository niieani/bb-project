#!/usr/bin/env bash
set -euo pipefail

version=""
sha256=""
repository=""
output=""

while (($#)); do
  case "$1" in
    --version) version="${2:?missing version}"; shift 2 ;;
    --sha256) sha256="${2:?missing sha256}"; shift 2 ;;
    --repository) repository="${2:?missing repository}"; shift 2 ;;
    --output) output="${2:?missing output}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$version" || -z "$sha256" || -z "$repository" || -z "$output" ]]; then
  echo "usage: $0 --version VERSION --sha256 SHA256 --repository OWNER/REPO --output PATH" >&2
  exit 2
fi
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid version: $version" >&2
  exit 2
fi
if [[ ! "$sha256" =~ ^[0-9a-f]{64}$ ]]; then
  echo "invalid sha256: $sha256" >&2
  exit 2
fi
if [[ ! "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "invalid repository: $repository" >&2
  exit 2
fi

mkdir -p "$(dirname "$output")"
cat >"$output" <<CASK
cask "bb-menubar" do
  version "$version"
  sha256 "$sha256"

  url "https://github.com/$repository/releases/download/v#{version}/BBMenuBar_#{version}_macOS.zip"
  name "BB Menu Bar"
  desc "Native menu bar status for bb-managed Git repositories"
  homepage "https://github.com/$repository"

  depends_on cask: "bb"
  depends_on macos: ">= :sonoma"

  app "BBMenuBar.app"

  zap trash: "~/Library/Preferences/dev.niieani.bb-menubar.plist"
end
CASK
