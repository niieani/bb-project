# Plan: Pluggable Notifications + launchd Scheduler (osascript first)

## Summary

Implement a pluggable notification layer with initial backends `stdout` and `osascript`, keep CLI default behavior as `stdout`, and add a new scheduler command group that installs a macOS LaunchAgent configured to run `bb sync --notify` with `osascript`. Persist notification delivery failures and surface them as warnings in `bb doctor` (exit code remains `0` unless unsyncable repos exist).

## Locked Decisions

1. Backend selection is singleton.
2. Selection precedence is CLI flag over environment variable.
3. Environment variable will be `BB_NOTIFY_BACKEND`.
4. `stdout` is treated as a real backend.
5. Default backend for `bb sync` is always `stdout` when no flag/env override is present.
6. Add scheduler commands now.
7. Scheduler install is replace-in-place when already installed.
8. Scheduler default backend is `osascript`.
9. Scheduler cadence is configurable via YAML and `bb config`; default is 60 minutes.
10. Delivery failure is persisted and shown in `bb doctor`; `bb doctor` exits `0` with warning when that is the only issue.
11. Persisted delivery warning is cleared on the next successful send.

## Public API / Interface / Type Changes

1. CLI (`/Volumes/Projects/Software/bb-project/internal/cli/cli.go`)

- Extend `bb sync` with `--notify-backend <stdout|osascript>`.
- Add `bb scheduler` command group:
- `bb scheduler install`
- `bb scheduler status`
- `bb scheduler remove`

2. App interface (`/Volumes/Projects/Software/bb-project/internal/cli/cli.go` appRunner + `/Volumes/Projects/Software/bb-project/internal/app/app.go`)

- Add methods:
- `RunSchedulerInstall(opts SchedulerInstallOptions) (int, error)`
- `RunSchedulerStatus() (int, error)`
- `RunSchedulerRemove() (int, error)`
- Extend `SyncOptions` with `NotifyBackend string`.
- Add `SchedulerInstallOptions` type with `NotifyBackend string`.

3. Config schema (`/Volumes/Projects/Software/bb-project/internal/domain/types.go`)

- Add new section:
- `scheduler.interval_minutes` (int, required-by-defaulted config)
- Keep `notify` section for delivery behavior only.

4. Notify runtime state (`/Volumes/Projects/Software/bb-project/internal/domain/types.go` + `/Volumes/Projects/Software/bb-project/internal/state/store.go`)

- Extend notify cache to include persisted delivery failures keyed by repo+backend.
- Include failure timestamp and error summary.
- Clear failure entry on successful send for same key/backend.

5. Notifier abstraction (`/Volumes/Projects/Software/bb-project/internal/app/sync_phase_notify.go` and new app file)

- Add backend interface and registry/factory:
- `stdout` backend: current line output behavior.
- `osascript` backend: invoke `osascript` with escaped title/body/subtitle payload.

## Implementation Plan

1. Introduce notifier abstraction and backend resolver

- Create app-layer types for notification event and backend interface.
- Implement backend resolver with precedence:
- `SyncOptions.NotifyBackend` if set
- `BB_NOTIFY_BACKEND` if set
- default `stdout`
- Validate backend names and return usage/hard error for invalid value.

2. Refactor notify flow to use backend interface

- In `/Volumes/Projects/Software/bb-project/internal/app/sync_phase_notify.go`, keep current dedupe/throttle logic unchanged.
- Replace direct stdout write with backend `Send`.
- On send failure:
- persist failure record
- continue processing remaining repos
- do not update `last_sent` for failed deliveries
- On success:
- update `last_sent`
- clear matching persisted failure warning entry.

3. Add persisted warning reporting in doctor

- In `/Volumes/Projects/Software/bb-project/internal/app/app.go` (`RunDoctor`), after existing unsyncable reporting:
- load persisted notify delivery failures
- print warning lines with backend, repo key/name/path marker, timestamp, and error summary
- keep exit code behavior:
- return `1` only for unsyncable repos
- return `0` if only delivery warnings are present.

4. Add scheduler config model + validation

- Update `/Volumes/Projects/Software/bb-project/internal/domain/types.go` and `/Volumes/Projects/Software/bb-project/internal/state/store.go` defaults:
- `scheduler.interval_minutes: 60`
- Update `/Volumes/Projects/Software/bb-project/internal/app/config.go` validation:
- interval must be `>= 1`.

5. Extend config wizard for scheduler interval

- Update `/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`:
- add a Scheduler step with integer input for interval minutes
- wire into step navigation, validation, diff summary, and dirty tracking
- Update tests in `/Volumes/Projects/Software/bb-project/internal/app/config_wizard_test.go`.

6. Implement scheduler app operations

- Add scheduler logic in new app file (for separation):
- macOS-only guard (`runtime.GOOS == "darwin"`), otherwise return exit `2` with clear message.
- LaunchAgent path: `~/Library/LaunchAgents/com.bb-project.sync.plist`.
- Install behavior:
- resolve absolute `bb` binary path (`os.Executable`)
- read `scheduler.interval_minutes` from config
- choose backend:
- explicit install option backend if provided
- else `osascript`
- create plist with `RunAtLoad=true`, `StartInterval=interval*60`, and program args:
- `<bb> sync --notify --quiet --notify-backend <backend>`
- standard logs in local state dir
- replace in place:
- attempt unload/bootout old job if present
- write plist
- load/bootstrap new job
- Status behavior:
- report plist existence, configured interval/backend from plist content, and loaded state via `launchctl print`.
- Remove behavior:
- unload/bootout job if loaded
- delete plist file
- return success if already absent.

7. Add CLI command wiring

- Update `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`:
- add sync flag forwarding for `--notify-backend`
- add `scheduler` command tree and handlers
- update fake app and parser tests in `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`.

8. Update docs and generated command docs

- Update `/Volumes/Projects/Software/bb-project/README.md` with:
- notify backend selection (`--notify-backend`, `BB_NOTIFY_BACKEND`)
- scheduler setup flow using `bb scheduler install`
- note that changing `scheduler.interval_minutes` requires re-running install.
- Regenerate docs/manpages via `go run ./cmd/bb-docs`:
- `/Volumes/Projects/Software/bb-project/docs/cli/*`
- `/Volumes/Projects/Software/bb-project/docs/man/man1/*`
- Update spec/doc note in `/Volumes/Projects/Software/bb-project/docs/implemented/001-SPEC.md` to reflect explicit scheduler command lifecycle.

## Test Plan (TDD-first)

1. Unit tests: backend resolver and notify behavior

- Invalid backend returns error.
- Precedence: CLI flag overrides env var.
- Default resolver returns `stdout`.
- `stdout` backend emits existing `notify <repo>: <fingerprint>` format.
- `osascript` backend command builder escapes payload safely.
- Failure persists warning and does not write sent cache.
- Subsequent success clears warning entry.

2. Unit tests: doctor warning behavior

- Doctor prints persisted notify delivery warnings.
- Doctor exits `0` when only warnings exist.
- Doctor still exits `1` when unsyncable repos exist.

3. CLI tests

- `sync --notify --notify-backend osascript` forwards option.
- Unknown backend value fails with exit `2`.
- Scheduler subcommands parse and dispatch correctly.

4. Scheduler unit tests (mocked command runner, no real launchctl dependency)

- Install writes expected plist (interval/backend/args).
- Install replace-in-place sequence invokes unload/load.
- Status reads installed state and reports fields.
- Remove handles existing and absent plist cleanly.
- Non-macOS behavior returns clear unsupported error.

5. Config tests

- Default config includes scheduler interval 60.
- Validation rejects `scheduler.interval_minutes < 1`.
- Config wizard edits and persists scheduler interval correctly.

6. E2E tests

- Existing notify e2e remain valid with default `stdout`.
- Add scheduler e2e smoke guarded to darwin only:
- install writes plist with expected args
- remove cleans plist.

## Assumptions and Defaults

1. Backend set is intentionally minimal for now: `stdout`, `osascript`.
2. Scheduler install default backend is `osascript`; interactive CLI sync default remains `stdout`.
3. `bb doctor` warnings for delivery failures are informational and do not alter exit success when no unsyncable repos exist.
4. Scheduler interval is configuration-driven; runtime launchd interval is not auto-reconciled after config edits, so users re-run `bb scheduler install` after interval changes.
5. Action buttons in notifications are out of scope for this phase because `osascript display notification` does not support custom actions.
