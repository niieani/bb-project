# bb v1.1 Hardening Plan

## Scope

This plan addresses three concrete v1 gaps:

1. Attention delivery throttling was not enforced.
2. Local lock handling has no stale lock recovery.
3. Sync orchestration in `internal/app/sync.go` is too monolithic for safe iteration.

The goal is to improve reliability and maintainability without changing v1 core behavior (winner selection, syncable rules, state schema versioning, external state transport model).

## Non-goals

- No change to the external state transport contract.
- No change to repo winner semantics.
- No major CLI surface redesign.
- No distributed/global lock (this remains per-machine local locking).

## Deliverables

1. Notification throttle implemented and tested.
2. Stale lock recovery implemented and tested.
3. Sync orchestration refactored into phase-oriented units with behavior parity tests.
4. Updated docs for new lock behavior and notification semantics.

## Milestone 1: Notification Throttle Enforcement

### Problem

Native notification throttling is now owned by BBMenuBar using the Go-exported attention policy.

### Plan

1. Go exports the eligible fleet fingerprint and throttle duration.
2. The native app persists the last submitted fingerprint and timestamp.
3. Changed fingerprints wait for the throttle boundary; unchanged fingerprints never repeat.

### Tests

Swift external-behavior tests cover first delivery, deduplication, reset, changed-set throttling, persistence, and failure states.

### Acceptance Criteria

- Native digest delivery enforces Go-provided `attention.throttle_minutes`.
- Tests use an injected clock and persistence seam.

## Milestone 2: Stale Lock Recovery

### Problem

Lock file handling in `internal/state/store.go` uses `O_CREATE|O_EXCL` only. If the process crashes, lock file can remain forever and block all future commands.

### Plan

1. Expand lock file payload to structured key/value lines, for example:
   - `pid=<pid>`
   - `hostname=<hostname>`
   - `created_at=<rfc3339>`
2. Add stale detection during lock acquisition:
   - If lock exists and PID is not alive on the same host, treat as stale.
   - If lock metadata is invalid/unparseable, use file age fallback.
   - Add max lock age guard (for example `24h`) as final stale fallback.
3. Recover by removing stale lock and retrying acquisition once.
4. Keep user-facing behavior unchanged for active lock contention (`another bb process holds the lock`).

### Tests

Add/extend tests in `internal/state` and `internal/e2e`:

- Active lock held by running process still blocks.
- Lock with nonexistent PID is recovered automatically.
- Corrupt lock file older than stale threshold is recovered.
- Recent corrupt lock still blocks (safety-first).

### Acceptance Criteria

- Crash leftovers no longer permanently block `bb`.
- Live process lock is never stolen.
- Lock recovery logic is deterministic and covered by tests.

## Milestone 3: Sync Orchestration Refactor

### Problem

`internal/app/sync.go` currently mixes discovery, local observation, persistence, winner reconciliation, cloning/adoption, and notifications in one large flow. This increases change risk and review complexity.

### Plan

Refactor with no intended behavior change by splitting into phase-oriented units:

1. `sync_orchestrator.go`
   - top-level `runSync` flow and phase sequencing.
2. `sync_phase_observe.go`
   - catalog selection, repo discovery, local observation/apply logic.
3. `sync_phase_reconcile.go`
   - winner selection, target validation, ensure/adopt local copy.
4. `sync_phase_publish.go`
   - machine record persistence helpers.
5. `status_contract.go`
   - fleet attention eligibility, fingerprint, and exported throttle policy.

Implementation notes:

- Keep functions package-private in `app` package first (no API expansion).
- Keep current data structures to avoid broad churn.
- Prefer extracting pure helpers for branch/path decisions.

### Tests

1. Keep existing e2e suite as behavior parity guard.
2. Add focused unit tests for extracted pure helpers (if introduced).
3. Ensure no snapshot/state file format regressions.

### Acceptance Criteria

- `go test ./...` passes with unchanged sync behavior.
- `runSync` reads as a short phase pipeline.
- New sync changes can be made by editing isolated phase files.

## Suggested Work Chunk Sequence

1. Work Chunk-1: Notification throttle + tests.
2. Work Chunk-2: Stale lock recovery + tests.
3. Work Chunk-3: Sync refactor (no behavior changes) + parity test run.

This sequence keeps risk low and simplifies rollback.

## Validation Checklist

- Run full test suite: `go test ./...`.
- Run focused e2e suites:
  - `go test ./internal/e2e -run TestNotifyCases -count=1`
  - `go test ./internal/e2e -run TestSyncBasicCases -count=1`
  - `go test ./internal/e2e -run TestSyncEdgeCases -count=1`
- Manual smoke (optional):
  - Simulate stale lock file, confirm automatic recovery.
  - Simulate repeated aggregate attention digests across the throttle window.

## Risks and Mitigations

- Risk: lock recovery could steal a valid lock.
  - Mitigation: require strong stale evidence (dead PID and/or age threshold); never steal recent ambiguous locks.
- Risk: throttle semantics may hide useful alerts.
  - Mitigation: document behavior clearly and allow `throttle_minutes: 0`.
- Risk: refactor introduces subtle sync regressions.
  - Mitigation: phase-by-phase extraction with no logic changes and full e2e parity runs.

## Definition of Done

- All three milestones implemented.
- Tests added for new behavior and passing in CI/local.
- Docs updated (`SPEC`/ops notes if needed) to reflect throttle and lock recovery semantics.
- No regressions in sync/init/scan/status workflows.
