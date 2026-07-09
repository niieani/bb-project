# Spec: Signal Repair — state tiers, quiet notifications, cross-machine visibility

## Goal

Make `bb`'s automatic sync trustworthy without changing what it is allowed to touch:

1. Stop flagging normal WIP as a problem — replace the `syncable` boolean with a
   tiered repo state (`synced` / `pending` / `wip` / `blocked`).
2. Stop freezing convergence on cosmetic remote-URL drift — demote
   `remote_format_mismatch` to a warning with a verified, revert-on-failure autofix.
3. Invert the notification model — one aggregated, activity-aware notification stream;
   WIP escalates only after 24h of inactivity; blockers are the only immediate class.
4. Give trust a surface — `bb overview` (cross-machine repo matrix), a sync journal
   (`bb log`), an extended `bb status --json`, and a menubar plugin over it.

Explicitly out of scope (deferred Phase 2): any form of automatic WIP
synchronization. `bb` still never mutates a dirty repo.

## Locked Decisions

1. **Breaking schema change, no migration.** `MachineFile.Version` bumps `1 -> 2`.
   Records written by older `bb` are not converted; `LoadAllMachineFiles` skips
   version-1 files with a warning naming the stale machine ("machine X publishes
   old-format state; update bb there"). Legacy fields (`syncable`,
   `unsyncable_reasons`) are removed, not kept alongside.
2. **Winner selection semantics unchanged**: only `state == synced` observations can
   win; local convergence write-actions still require local `state == synced`.
   This spec changes classification and reporting, not convergence safety.
3. **`remote_format_mismatch` no longer affects state.** It becomes a repo *warning*.
   Autofix runs during sync, gated by `sync.auto_align_remote_format` (default
   `true`), and persists only after verification succeeds; otherwise it reverts to
   the previous URL and records a warning.
4. **Notification stream is a single aggregated message per sync run**, deduped by a
   fingerprint of the whole attention set. Per-repo notifications are removed.
   `notify.dedupe` config key is removed (always on).
5. **Activity staleness is per-machine observation**, computed at scan time from
   filesystem mtimes (never compared across machines' clocks beyond display).
6. **Reason classification** (see table below) is fixed by this spec; new reasons
   must declare a tier when introduced.
7. **`bb fix` is adapted, not redesigned**: grouping/labels follow the new tiers;
   no new TUI investment in this change.
8. Journal lives in the shared state dir (visible cross-machine), pruned on write.

## State model

### Repo state (per machine observation)

```go
type RepoSyncState string

const (
    RepoStateSynced  RepoSyncState = "synced"  // clean; participates in winner selection + convergence
    RepoStatePending RepoSyncState = "pending" // bb knows the remediation but policy/config gates it
    RepoStateWip     RepoSyncState = "wip"     // human is (or was) mid-work; informational
    RepoStateBlocked RepoSyncState = "blocked" // needs a human decision; immediate-notify class
)
```

### Reason classification

| Reason | Tier | Rationale |
| --- | --- | --- |
| `dirty_tracked` | wip | normal mid-work |
| `dirty_untracked` | wip | normal mid-work (rule still gated by `sync.include_untracked_as_dirty`) |
| `operation_in_progress` | wip | mid-rebase/merge is work, not failure; escalates via staleness if abandoned |
| `missing_upstream` | wip | new local branch not yet published |
| `missing_origin` | wip | new local project not yet published (`bb init`/`create-project` pending) |
| `push_policy_blocked` | wip | unpublished commits; policy says don't auto-push |
| `diverged` | blocked | needs a merge/rebase decision |
| `push_access_blocked` | blocked | ahead on a read-only remote; needs fork/permission decision |
| `sync_conflict_requires_manual_resolution` | blocked | reconcile-time failure |
| `push_failed`, `pull_failed`, `checkout_failed`, `sync_feasibility_probe_failed` | blocked | action attempted and failed |
| `target_path_nonempty_not_repo`, `target_path_repo_mismatch` | blocked | path conflict |
| `clone_required` | pending | catalog policy (`auto_clone_on_sync=false`) gates it |
| `catalog_not_mapped`, `catalog_mismatch` | pending | config/`move-to-catalog` gates it |
| `remote_format_mismatch` | — (warning) | cosmetic; never a state |

State derivation: `blocked` if any blocked reason, else `pending` if any pending
reason, else `wip` if any wip reason, else `synced`.

### Domain API

`EvaluateSyncability` is replaced (removed, not wrapped):

```go
// internal/domain/state_eval.go
func EvaluateRepoState(state ObservedRepoState, autoPush bool, cliPush bool) RepoStateResult

type RepoStateResult struct {
    State   RepoSyncState
    Reasons []UnsyncableReason // retains existing reason constants; rename of the type is out of scope
}

func ReasonTier(r UnsyncableReason) RepoSyncState // wip|pending|blocked; panics on unknown (explicit failure)
```

`IsBlockingUnsyncableReason` / `HasBlockingUnsyncableReason` are removed; call sites
use `ReasonTier(...) == RepoStateBlocked` / `State == RepoStateBlocked`.

`SelectWinner` filters on `Record.State == RepoStateSynced`.

### Schema changes (`MachineRepoRecord`, version 2)

```yaml
# removed
syncable: bool
unsyncable_reasons: [...]

# added
state: synced|pending|wip|blocked
reasons: [...]            # same reason strings; empty when synced
warnings: [remote_format_mismatch, ...]   # cosmetic, never affects state
last_activity_at: <time>  # see below; zero value allowed when unknown
```

`state_hash` covers `state`, `reasons`, and `warnings`; it must NOT cover
`last_activity_at` (mtime churn must not touch `observed_at`, or winner selection
would flap).

### `last_activity_at` (scan-time)

Computed per repo during observation as the max of:

- mtime of `.git/HEAD` and `.git/index` (captures commits, checkouts, staging)
- mtimes of paths reported dirty/untracked by the already-executed `git status
  --porcelain` (stat only those paths; no tree walk)

Stat errors on individual paths are skipped (a dirty path can vanish between status
and stat); if every probe fails, `last_activity_at` stays zero and staleness
escalation treats the repo as stale (explicit, documented behavior — an unknown
activity time must not permanently suppress escalation).

## Remote format: warning + verified autofix

Scan (`internal/app/app.go` remote-format check): on template mismatch, append
`remote_format_mismatch` to `warnings`; never touch `state`.

Sync gains an align phase (before observe/publish so the corrected URL is what gets
recorded), per repo with the warning, when `sync.auto_align_remote_format` is true:

1. `git remote set-url origin <preferred>`
2. verify: `git ls-remote --heads origin` (bounded timeout, reuses existing command
   runner seam)
3. on success: keep; journal `remote_aligned`
4. on failure: `git remote set-url origin <previous>`; journal `remote_align_reverted`
   with the verification error; warning remains

The manual `align-remote-format` fix action stays (same verified implementation,
shared function). Repos with only this warning are otherwise `synced` and converge
normally even when autofix is disabled or failing.

## Notification policy

Replaces the per-repo pipeline in `sync_phase_notify.go` entirely.

**Attention set** (computed after reconcile, per sync run):

- every repo with `state == blocked`, unless in quiet period
  (`now - last_activity_at < notify.quiet_hours`, default 2h — you were just in that
  repo; you know)
- every repo with `state == wip` whose `last_activity_at` is older than
  `notify.wip_stale_hours` (default 24h)
- `pending` repos never notify (visible in status/overview only)

**Delivery**: one notification per run, only when the attention-set fingerprint
(sorted `repo_key:state:reasons` lines) differs from the last sent one, and outside
`notify.throttle_minutes` (kept, default 60). Body: `N repo(s) need attention:` +
up to 4 repo names with their dominant reason, `+K more` overflow.

**Cache** (`NotifyCacheFile` version bump, old file discarded on load if
version < 2): single `last_sent {fingerprint, sent_at}` entry replaces the per-repo
map. `DeliveryFailures` keeps its existing shape and doctor surfacing but keyed by
backend only.

Config:

```yaml
notify:
  enabled: true
  throttle_minutes: 60
  quiet_hours: 2        # new
  wip_stale_hours: 24   # new
  # dedupe: removed
```

## Sync journal + `bb log`

Append-only JSONL per machine in the shared state dir:
`<state>/journal/<machine_id>.jsonl`, pruned on write to `journal.max_entries`
(default 500).

Event shape:

```json
{"at":"...","machine":"...","event":"converged","repo_key":"projects/x","detail":"checkout main + ff 3"}
```

Events: `sync_run` (summary: counts per state, duration), `converged`, `cloned`,
`pushed`, `remote_aligned`, `remote_align_reverted`, `notified`, `fix_applied`.
Writes go through the existing store (lock already held during sync); journal write
failures are logged, never fail the sync (observability must not break the thing it
observes).

`bb log [--repo <selector>] [--machine <id>] [--limit N] [--json]` merges all
machines' journals, newest first. Default limit 50.

## `bb overview`

New command: cross-machine matrix from `LoadAllMachineFiles` + repo metadata.

Plain output, one line per repo (repos where all machines are `synced` are collapsed
into a count unless `--all`):

```
projects/gpt-tokenizer   here: wip (dirty 2h ago)   MiniPC: synced   MBPAir: — (not cloned)
projects/scaffold        here: blocked (diverged)   MiniPC: wip (dirty 6d ago)
synced everywhere: 61 repos (--all to list)
```

- `here` column first, then other machines by id.
- Each cell: state + dominant reason + humanized `last_activity_at` for wip/blocked.
- Machine-level staleness banner when a machine file's `updated_at` is older than
  `overview.machine_stale_days` (default 3): "MiniPC last published 5d ago — its data
  may be stale."
- `--json` emits the full matrix (all repos, all machines, reasons, warnings,
  timestamps) — this JSON is the menubar plugin's data source.
- Flags: `--include-catalog` (repeatable), `--all`.
- Exit code 0 always (it is a view, not a check; `doctor` keeps the check role).

## `bb status` / `bb doctor` / exit codes

- `status` plain: per-repo `state` (+ reasons for non-synced) and a summary line:
  `87 repos: 61 synced · 3 pending · 21 wip · 2 blocked · 19 warnings`.
- `status --json`: adds `state`, `reasons`, `warnings`, `last_activity_at`, the
  summary object, and `last_sync` (from the journal's latest `sync_run` of this
  machine).
- `doctor`: groups output by tier (blocked first, then stale wip, then pending, then
  warnings). Exit `1` only when a `blocked` repo exists in selected catalogs
  (previously: any blocking unsyncable reason). Stale-wip and warnings exit `0`.
- `sync`/`ensure`: exit `1` only for remaining `blocked` repos.

## Menubar plugin (SwiftBar/xbar)

`contrib/bb-menubar/bb.10m.sh` — shell + `jq` over `bb status --json` and
`bb overview --json`:

- title: `✓ 87` when nothing blocked/stale; `⚠ 2` otherwise
- dropdown: blocked repos, stale-wip repos, other machines' dirty repos, last sync
  time, `bb sync` trigger line
- README section documents installation (copy/symlink into SwiftBar plugins dir)

Shipped as contrib script; no Go code, no test harness beyond shellcheck in CI if
already present (do not add new CI infra for this).

## `bb fix` adaptation (minimal)

- List grouping/order: `blocked`, stale `wip`, `pending`, fresh `wip`, `synced`,
  ignored. Labels use tier names; "unsyncable" wording disappears from the UI.
- Eligibility logic unchanged; it keys off reasons, which are preserved.
- `align-remote-format` action reads the warning instead of the removed reason.

## Config summary (all changes)

```yaml
sync:
  auto_align_remote_format: true   # new
notify:
  quiet_hours: 2                   # new
  wip_stale_hours: 24              # new
  # dedupe removed
journal:
  max_entries: 500                 # new section
overview:
  machine_stale_days: 3            # new section
```

Config wizard: add toggles/inputs for the new keys; remove the dedupe toggle.
`ConfigFile.Version` stays 1 (additive keys, removed key ignored on load — config is
per-machine and regenerated by the wizard; no cross-machine coupling).

## Docs

- Update `README.md` (states, overview, log, notify semantics, menubar).
- Update `docs/implemented/001-SPEC.md` terminology via a pointer note; this doc
  moves to `docs/implemented/016-SIGNAL_REPAIR.md` on completion.

## TDD slices

Each slice: red tests first, then implement, `just build` + full test run green
before the next.

1. **Domain state tiers**: `RepoSyncState`, `ReasonTier`, `EvaluateRepoState`
   (replaces `EvaluateSyncability` + `IsBlockingUnsyncableReason`); `SelectWinner`
   on `state`; `statehash` covers state/reasons/warnings, excludes
   `last_activity_at`. Tests: tier table, derivation precedence
   (blocked > pending > wip), winner filtering, hash stability under mtime change.
2. **Schema v2**: `MachineRepoRecord` new fields, `MachineFile.Version = 2`,
   `LoadAllMachineFiles` skips v1 files with named warning. Tests: round-trip, v1
   skip + warning text, reconcile ignores skipped machines.
3. **Scan `last_activity_at`**: mtime probe (HEAD, index, dirty paths), zero-value
   on total failure. Tests: harness fixtures with controlled mtimes; dirty-path
   vanish race.
4. **Remote-format warning + verified autofix**: scan writes warning not state; sync
   align phase (apply -> `ls-remote` verify -> revert), config gate, journal events;
   shared implementation with fix action. Tests: success persists, verify-failure
   reverts and keeps warning, disabled gate no-ops, e2e repo converges with warning
   present.
5. **Notify rewrite**: attention set (blocked minus quiet-period, stale wip),
   aggregated single message, set-fingerprint dedupe, throttle, cache v2 (discard
   old), config keys. Tests: churn during active work sends nothing; 24h-stale wip
   enters digest; blocked in quiet period suppressed; overflow formatting; cache
   migration-by-discard.
6. **Journal + `bb log`**: event writes from sync/fix paths, pruning, merge/sort/
   filters, write-failure tolerance. Tests: pruning boundary, merged ordering,
   `--repo`/`--machine` filters, sync survives journal write error.
7. **Status/doctor/exit codes**: tier grouping, summary line, JSON extension with
   `last_sync`, doctor exit only on blocked. Tests: golden outputs, exit codes per
   tier mix.
8. **`bb overview`**: matrix assembly, collapse rule, machine-staleness banner,
   `--json`, selector flags. Tests: multi-machine fixtures (existing e2e harness has
   multi-machine setup), not-cloned cells, stale machine banner.
9. **`bb fix` adaptation + config wizard fields + contrib menubar script + docs.**
   Tests: fix list grouping golden, wizard field round-trip.

Slices 1–3 land together as one PR-sized unit if intermediate states won't compile
cleanly (schema and evaluation are coupled); 4–9 are independent after that.

## Risks / notes

- All machines must update `bb` near-simultaneously after schema v2 lands (v1
  machine files are skipped, so an un-updated machine silently stops participating
  in reconcile until updated — the named warning in sync/doctor output mitigates).
- `ls-remote` verification needs network; offline sync runs leave the warning in
  place and retry next run (acceptable — align is idempotent).
- `last_activity_at` from mtimes is a heuristic (build tools touching files count as
  activity). Acceptable: false "active" only delays escalation; false "stale" is
  bounded by the 24h window.
