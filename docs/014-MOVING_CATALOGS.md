# 014: Moving Repositories Across Catalogs

## Summary
Add first-class support for moving a repository from one catalog to another while preserving cross-machine convergence.

Primary outcomes:
- local command to move a repo and update shared metadata
- cross-machine detection of stale local placement as `catalog_mismatch`
- `bb fix` remediation action: `move-to-catalog`
- optional post-move hooks that run on every machine where a move executes

Implementation checklist: `docs/014-MOVING_CATALOGS-CHECKLIST.md`.

## Motivation
Current sync flow treats `repo_key` as catalog/path authority, but there is no supported way to migrate that authority when a repo should live in a different catalog. Without explicit move semantics, machines can drift and metadata can be recreated under legacy keys.

## Goals
- provide explicit move workflow in CLI
- propagate move intent via synced metadata
- surface mismatch state as non-blocking unsyncable reason
- provide deterministic remediation in `bb fix`
- support configurable post-move local hooks

## Non-Goals
- automatic migration of catalogs that are not configured locally
- automatic cloning for machines that never had the repository locally when move history exists
- preserving legacy move mechanisms

## Data Model Changes
- Add unsyncable reason:
  - `catalog_mismatch` (non-blocking)
- Extend `RepoMetadataFile`:
  - `previous_repo_keys: []string`
- Extend `ConfigFile`:
  - `move.post_hooks: []string`
- Extend `MachineRepoRecord` (optional expected-target hints):
  - `expected_repo_key`, `expected_catalog`, `expected_path`

## CLI Changes
New command:
- `bb repo move <repo> --catalog <target> [--as <catalog-relative-path>] [--dry-run] [--no-hooks]`

Behavior:
- resolves `<repo>` using existing local selector conventions (`path`, exact `repo_key`, unique name)
- validates target path safety using existing conflict checks
- moves local directory (rename; copy+remove fallback across devices)
- rewrites metadata to the new `repo_key`
- appends old key to `previous_repo_keys`
- removes old metadata file
- re-observes and persists local machine record at new path
- runs post-move hooks unless `--no-hooks`

## Sync / Fix Behavior
### Sync
- Build old-key -> new-key move index from `previous_repo_keys`.
- If local repo is on an old key:
  - mark `catalog_mismatch`
  - include expected target hints when resolvable
  - include `catalog_not_mapped` if target catalog missing locally
- For moved repos with no local clone on a machine, do not synthesize `clone_required`.

### Fix
- New action: `move-to-catalog`
- Eligible when reason includes `catalog_mismatch`.
- Action moves repo to expected destination and runs post-move hooks.
- If expected target cannot be resolved (for example unmapped target catalog), action is blocked with remediation guidance.

## Post-Move Hooks
Config:
```yaml
move:
  post_hooks:
    - my-command --old "$BB_MOVE_OLD_PATH" --new "$BB_MOVE_NEW_PATH"
```

Environment variables:
- `BB_MOVE_OLD_REPO_KEY`
- `BB_MOVE_NEW_REPO_KEY`
- `BB_MOVE_OLD_CATALOG`
- `BB_MOVE_NEW_CATALOG`
- `BB_MOVE_OLD_PATH`
- `BB_MOVE_NEW_PATH`

Execution model:
- runs after move state is persisted
- non-zero hook exits fail the command, but move remains applied

## Failure Semantics
- Target path conflict: fail move with existing path-reason details.
- Unknown target catalog: fail with actionable message.
- Ambiguous old-key mapping in move history: fail hard and require metadata repair.
- Missing target catalog on receiving machine: remain non-blocking (`catalog_mismatch` + `catalog_not_mapped`).

## TDD Plan
Detailed implementation order, exact test names, and file-by-file tasks are maintained in:
- `docs/014-MOVING_CATALOGS-CHECKLIST.md`

