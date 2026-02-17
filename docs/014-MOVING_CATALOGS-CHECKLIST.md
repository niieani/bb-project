# 014 Moving Catalogs Implementation Checklist

## Scope
Implement repository catalog moves that propagate safely across machines, surface `catalog_mismatch`, provide `bb repo move`, and support remediation via `bb fix move-to-catalog`.

## TDD Order
1. Add red tests for domain/model additions.
2. Add red tests for CLI surface (`bb repo move`).
3. Add red tests for app-level move execution and hooks.
4. Add red tests for sync mismatch detection and no-resurrection behavior.
5. Add red tests for fix action eligibility/execution (`move-to-catalog`).
6. Add red e2e tests for multi-machine propagation and remediation.
7. Implement incrementally until all tests pass.
8. Update docs and run doc generator.

## Exact Test Cases

### Domain/State
- `internal/domain/unsyncable_reason_test.go`
  - `TestHasBlockingUnsyncableReason`
    - add assertion that `catalog_mismatch` is non-blocking.

- `internal/state/store_test.go`
  - `TestLoadConfigDefaults`
    - verify `move.post_hooks` default is empty.
  - `TestSaveLoadConfigRoundTripMoveHooks`
    - verify `move.post_hooks` round-trip.

### CLI
- `internal/cli/cli_test.go`
  - `TestRepoMoveCommandForwardsOptions`
    - `bb repo move software/api --catalog references --as tools/api --dry-run --no-hooks`
    - assert forwarded `RepoMoveOptions`.
  - `TestRepoMoveCommandRequiresCatalog`
    - missing `--catalog` should return CLI usage error.

### App Unit
- `internal/app/repo_move_test.go`
  - `TestRunRepoMoveMovesRepoAndRewritesMetadata`
  - `TestRunRepoMoveAppendsPreviousRepoKeys`
  - `TestRunRepoMoveRejectsTargetPathConflict`
  - `TestRunRepoMoveDryRunDoesNotMutate`
  - `TestRunRepoMoveRunsPostMoveHooks`
  - `TestRunRepoMoveNoHooksSkipsPostMoveHooks`
  - `TestRunRepoMoveFailsWhenTargetCatalogUnknown`

- `internal/app/repo_move_index_test.go`
  - `TestBuildRepoMoveIndex`
  - `TestBuildRepoMoveIndexRejectsAmbiguousOldKey`

### Sync / Observe / Fix Integration
- `internal/app/sync_orchestrator_test.go`
  - `TestAnyUnsyncableInSelectedCatalogsIgnoresNonBlockingReasons`
    - extend with `catalog_mismatch`.

- `internal/app/sync_phase_reconcile_test.go` (new)
  - `TestEnsureFromWinnersMarksCatalogMismatchForPreviousRepoKey`
  - `TestEnsureFromWinnersSkipsCloneRequiredForMovedRepoWhenNeverCloned`
  - `TestEnsureFromWinnersMarksCatalogNotMappedWhenMoveTargetCatalogMissing`

- `internal/app/fix_actions_test.go`
  - `TestEligibleFixActions`
    - case: `catalog_mismatch` => includes `move-to-catalog`.

- `internal/app/fix_apply_test.go`
  - `TestApplyFixActionMoveToCatalog`
  - `TestApplyFixActionMoveToCatalogRequiresExpectedTarget`

### E2E
- `internal/e2e/catalog_move_test.go` (new)
  - `TestCatalogMovePropagatesAsMismatchAndFixesWithMoveToCatalog`
  - `TestCatalogMoveNoopOnMachineWithoutLocalClone`
  - `TestCatalogMoveHooksRunOnInitiatorAndFixingMachine`

## File-by-File Implementation Plan

### Domain / State
- `internal/domain/types.go`
  - add `ReasonCatalogMismatch`.
  - add `MoveConfig` + `ConfigFile.Move`.
  - add `RepoMetadataFile.PreviousRepoKeys`.
  - add optional expectation fields to `MachineRepoRecord`:
    - `ExpectedRepoKey`, `ExpectedCatalog`, `ExpectedPath`.

- `internal/domain/unsyncable_reason.go`
  - classify `ReasonCatalogMismatch` as non-blocking.

- `internal/domain/statehash.go`
  - include expected-move fields in state-hash payload.

- `internal/state/store.go`
  - default config includes `move.post_hooks: []`.

### CLI
- `internal/cli/cli.go`
  - extend `appRunner` with `RunRepoMove(opts app.RepoMoveOptions) (int, error)`.
  - add `bb repo move <repo> --catalog <name> [--as <rel>] [--dry-run] [--no-hooks]`.

- `internal/cli/cli_test.go`
  - update fake app and add move command tests.

### App Core
- `internal/app/app.go`
  - add `RepoMoveOptions`.
  - implement `RunRepoMove` entrypoint.

- `internal/app/repo_move.go` (new)
  - implement planner + executor for local move.
  - handle same-device rename and cross-device fallback copy/remove.
  - rewrite metadata to new `repo_key`, append old key to history, remove old metadata file.
  - run post-move hooks with env:
    - `BB_MOVE_OLD_REPO_KEY`, `BB_MOVE_NEW_REPO_KEY`
    - `BB_MOVE_OLD_CATALOG`, `BB_MOVE_NEW_CATALOG`
    - `BB_MOVE_OLD_PATH`, `BB_MOVE_NEW_PATH`.

- `internal/app/repo_move_index.go` (new)
  - build old-key->current-key index from `previous_repo_keys`.
  - detect ambiguous old-key mappings.

### Observe / Sync
- `internal/app/app.go` (`observeRepo`)
  - prevent metadata resurrection for moved old keys.
  - mark observed records with `catalog_mismatch` when local key is historical.

- `internal/app/sync_phase_reconcile.go`
  - mark old-key local repos as `catalog_mismatch` (+ expected target fields).
  - if move target catalog missing locally, also mark `catalog_not_mapped`.
  - skip `clone_required` for moved repos on machines with no local copy.

### Fix
- `internal/app/fix.go`
  - add `FixActionMoveToCatalog` constant.
  - include in eligibility for `catalog_mismatch`.
  - ensure apply/revalidate path logic supports path-changing actions.
  - execute move action through shared move engine.

- `internal/app/fix_action_spec.go`
  - add label/description/plan for `move-to-catalog`.

- `internal/app/fix_tui.go`
  - map `catalog_mismatch` coverage to `move-to-catalog`.
  - reason labels / tier behavior update.

- `internal/app/fix_tui_wizard.go`
  - reason label and manual guidance for `catalog_mismatch`.

### Docs
- `docs/014-MOVING_CATALOGS.md`
  - include final behavior spec + decisions + migration notes.
  - link this checklist.

- `README.md`
  - add `bb repo move` command docs.
  - add `catalog_mismatch` to unsyncable reasons and fix flow.
  - document `move.post_hooks`.

- `docs/cli/*`, `docs/man/man1/*`
  - regenerate CLI/docs via docs generator after command additions.

## Validation Commands
- `go test ./internal/domain ./internal/state -count=1`
- `go test ./internal/cli ./internal/app -count=1`
- `go test ./internal/e2e -count=1`
- `go test ./... -count=1`
