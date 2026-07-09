# Research

## Problem report (owner)

- Owner does not trust automatic sync: no positive signal that convergence happened.
- macOS "Unsyncable" notifications fire constantly, including for repos being actively
  worked on (which are obviously dirty) => alert fatigue, notifications ignored.
- CLI triage workflow (`bb fix` table over all projects) is not a workflow the owner
  wants to perform regularly.
- Actual want: all machines converge to the same source of truth, nothing goes stale,
  and *before starting work locally*, know whether a repo is dirty on another machine.

## Live-state evidence (owner's primary machine, 2026-07-08)

- 87 repos tracked, **53 marked unsyncable**.
- Reason distribution (from `bb doctor`):
  - `remote_format_mismatch`: ~19 repos — purely cosmetic (origin URL does not match
    `github.preferred_remote_url_template`), yet freezes convergence entirely.
  - `dirty_untracked`: ~18, `dirty_tracked`: ~12 — normal WIP; permanent steady state
    for active repos (`sync.include_untracked_as_dirty: true`).
  - genuinely actionable (`diverged`, `missing_origin`): ~4 repos.
- Scheduler installed, 60 min interval, `osascript` backend.

## Root-cause analysis

1. **`Syncable` is a boolean that conflates normal WIP with real blockers.**
   `EvaluateSyncability` (`internal/domain/syncable.go`) returns unsyncable for dirty
   tracked/untracked, missing upstream (new local branch!), operation in progress
   (mid-rebase!), ahead-with-push-blocked — all normal mid-work states — alongside
   `diverged`/`missing_origin` which genuinely need a human.

2. **Cosmetic drift blocks convergence.** `internal/app/app.go:1205-1207` sets
   `Syncable=false` + `remote_format_mismatch` when origin URL != preferred template.
   The repo is then excluded from winner selection AND local convergence. A fix action
   (`align-remote-format`) exists but must be applied manually per repo.
   (`remote_format_mismatch` is already non-blocking for notify/exit-code via
   `IsBlockingUnsyncableReason`, but the `Syncable=false` side effect is the damage.)

3. **Notification model is inverted and context-blind.**
   `internal/app/sync_phase_notify.go`:
   - one notification per repo per fingerprint change; fingerprint = sorted reason set,
     which churns while working (`dirty_tracked` -> `+dirty_untracked` -> `+ahead`),
     re-notifying on every churn (dedupe only suppresses identical fingerprints).
   - no notion of "repo touched minutes ago => human is on it, stay quiet".
   - no positive signal anywhere ("all converged") => trust cannot form.

4. **Cross-machine data exists but has no read surface.** Machine files in the shared
   state dir already contain every machine's per-repo observations
   (`domain.MachineFile` / `MachineRepoRecord`), and `state.LoadAllMachineFiles`
   already loads them for reconcile. `bb status` only ever shows the local machine.

## Relevant code map

- Syncability evaluation: `internal/domain/syncable.go` (`EvaluateSyncability`)
- Reason constants + blocking split: `internal/domain/types.go`,
  `internal/domain/unsyncable_reason.go` (`IsBlockingUnsyncableReason`)
- Winner selection (requires `Syncable`): `internal/domain/winner.go`
- State hash: `internal/domain/statehash.go`
- Observation/scan incl. remote-format check: `internal/app/app.go` (~1150-1220)
- Notify pipeline + cache: `internal/app/sync_phase_notify.go`,
  `internal/app/notify_backend.go`, `state.Load/SaveNotifyCache`
  (`domain.NotifyCacheFile`)
- Machine/repo state schema: `internal/domain/types.go`
  (`MachineFile`, `MachineRepoRecord`, `ObservedRepoState`)
- Store/bootstrap/load-all: `internal/state/store.go`
- Status/doctor: `internal/app` (status, doctor + `doctor_notify_test.go`)
- Fix actions incl. `align-remote-format`: `internal/app/fix.go` (~300, ~480)
- Defaults: `internal/state/store.go:106` (`IncludeUntrackedAsDirty: true`),
  `:119` (`Notify{Enabled, Dedupe, ThrottleMinutes: 60}`)

## Owner decisions (from alignment Q&A, 2026-07-08)

1. **No automatic WIP sync** (neither WIP refs pushed to origin nor bundles over the
   state transport): risk of mid-work mess and accidental secret commits. Phase 2
   (explicit handoff) deferred until Phase 1 proves itself.
2. **`remote_format_mismatch`**: demote to warning; keep an automatic fix that is
   *verified before persisting* (apply -> verify remote reachability -> revert on
   failure).
3. **Trust surfaces wanted**: cross-machine visibility ("is this dirty on another
   machine before I start work locally?"), menubar indicator, sync log/history view.
4. **Staleness escalation**: dirty/WIP repo becomes notification-worthy after ~24h
   without local activity.
