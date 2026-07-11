# Design

## Contracts

Status attention items gain Go-computed local actions. Each action has stable kind/id/label and safety metadata. Swift renders supplied capabilities; it never derives eligibility from reasons or display text.

Mutations emit newline-delimited operation events: operation/repository start, phase/progress, repository completion, operation completion. Events carry repository key and concise user-facing message. Human CLI output remains separate.

## Runtime

Targeted sync uses the same sync engine and locking as global sync with an explicit repository selector. Fix invokes the existing action plan/executor only for capabilities classified safe and non-interactive.

Swift process execution reads events incrementally. A main-actor operation coordinator maintains one active mutation, per-project state, latest status line, and final error. Completion refreshes status/overview.

## UX

Trailing compact row controls. Spinner replaces/adjacent to the active row action. Footer shows one calm status line and global completion count where available. Disable competing mutations while the global lock is occupied. Preserve status dots plus textual reason: never color alone.

## Test seam

Primary seam: machine-visible CLI behavior consumed by `ProcessBBClient` and `MenuBarModel`. Go tests verify capability/event contracts and real repository effects. Swift tests use streams at `BBClient` to verify decode, state transitions, presentation, and error behavior. One live installed-app pass verifies final integration.

## Gates and acceptance

- Iteration: `gofmt`, focused Go/Swift tests.
- Slice completion: `just test`, `just build`, `swift test --package-path macos/BBMenuBar`; regenerate CLI docs with `just docs-cli` when flags change.
- Local native rebuild/reload: `./script/build_and_run.sh --verify`; this binds the app to the just-built CLI and avoids distribution signing/notarization.
- Release-shaped verification: `script/test_release_macos_app.sh` and unsigned universal packaging once at final acceptance. Universal arm64/x86_64 work is intentionally not part of each edit cycle.
- Computer acceptance: inspect live popup actions and refreshed real data. Transient spinner timing is authoritative in deterministic stream/model tests because real operations may complete before capture.

## Hazards

- Targeted sync must preserve unrelated machine records and constrain reconciliation; filtering discovery then publishing would lose state.
- Event stdout stays pure JSONL; stderr diagnostics drain concurrently; prior events survive a nonzero terminal exit.
- Capability calculation stays snapshot-derived during status and never adds network/risk probes to periodic refresh.
