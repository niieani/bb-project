## Goal

Make published macOS `bb` binaries Developer ID signed and notarized so the Homebrew cask installs cleanly under Gatekeeper.

## Design

- Keep the existing GoReleaser-based release architecture.
- Use GoReleaser's OSS `notarize.macos` support because `bb` ships standalone binaries, not `.app` bundles.
- Gate notarization on the presence of signing secrets so local config validation still works without Apple credentials.
- Use 1Password GitHub Actions plus the `op` CLI instead of storing the Apple materials directly as GitHub secrets.
- Put the shared publish logic in a `workflow_call` workflow that accepts the target ref and uses the same 1Password-backed secret loading path for both callers.
- Keep `.github/workflows/release-please.yml` focused on release PR/tag orchestration, then call the shared publish workflow only when `release-please` actually created a release.
- Keep `.github/workflows/release.yml` as a manual/tag-based recovery entry point, but make it call the same shared publish workflow instead of owning a second copy of the publish steps.
- Update the README release section to document the additional required secrets and the notarized cask outcome.

## Validation

- Run `goreleaser check` to validate the updated release configuration parses locally without the Apple secrets present.
- Confirm the workflow YAML still renders valid by inspecting the changed files after patching.
