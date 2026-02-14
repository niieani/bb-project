### Switch Repo Metadata Identity to Catalog Path Keys (Strict Depth) + Config Wizard Support

### Summary

Replace `repo_id`-keyed repo metadata persistence with a new path-based `repo_key` that is derived from catalog + folder structure.  
Implement strict per-catalog path depth (`1` or `2`) and update `bb config` interactive catalog management to edit it.  
This is a deliberate breaking change with no migration: old repo metadata files are ignored.

### Public API / Schema Changes

1. **Catalog schema**
   - File: `/Volumes/Projects/Software/bb-project/internal/domain/types.go`
   - Add `repo_path_depth` to `domain.Catalog`:
     - `1` = `catalogRoot/repo`
     - `2` = `catalogRoot/anything/repo`
   - Default when unset: `1`.

2. **Repo metadata schema**
   - File: `/Volumes/Projects/Software/bb-project/internal/domain/types.go`
   - Add `repo_key` to `domain.RepoMetadataFile`.
   - `repo_key` format: `<catalog>/<relative-path>`
     - Depth 1 example: `software/api`
     - Depth 2 example: `software/openai/codex`

3. **Machine record schema**
   - File: `/Volumes/Projects/Software/bb-project/internal/domain/types.go`
   - Add `repo_key` to `domain.MachineRepoRecord`.

4. **Unsyncable reasons**
   - Remove `duplicate_local_repo_id` behavior and constant.
   - File: `/Volumes/Projects/Software/bb-project/internal/domain/types.go`

### Core Behavioral Changes

1. **Metadata filename derivation**
   - File: `/Volumes/Projects/Software/bb-project/internal/state/store.go`
   - `repos/*.yaml` filename comes from `repo_key` (not `repo_id`).
   - Slash replacement remains `__`.
   - Example:
     - `software/api` -> `software__api.yaml`
     - `software/openai/codex` -> `software__openai__codex.yaml`

2. **Strict catalog depth**
   - For depth `1`, only repos exactly one directory below catalog root are managed.
   - For depth `2`, only repos exactly two directories below catalog root are managed.
   - Deeper or shallower repo roots are ignored for that catalog.
   - Applies to discovery and `init` target validation.

3. **Sync identity pivot**
   - Winner selection, local-match detection, metadata lookup, and reconcile grouping use `repo_key` (not `repo_id`).
   - Duplicate origin URLs are allowed as long as `repo_key` differs.

4. **Target path derivation**
   - Reconcile target path comes from `repo_key` relative path (portion after catalog name), not metadata `name`.

### Implementation Plan (Decision-Complete)

1. **Add repo-key/path-depth helpers in domain**
   - Add new helper module (e.g., `/Volumes/Projects/Software/bb-project/internal/domain/repo_key.go`) with:
     - Effective depth resolution (`default=1`).
     - Strict repo-key build from catalog root + repo path.
     - Repo-key parsing (`catalog`, `relative`, `name`).
   - Add unit tests for valid/invalid depth and parsing.

2. **Update validation**
   - File: `/Volumes/Projects/Software/bb-project/internal/app/config.go`
   - Extend machine validation:
     - `repo_path_depth` must be `1` or `2` (or omitted/0 -> default 1).
   - Keep absolute root and existing validations.

3. **Refactor state persistence API to repo_key**
   - File: `/Volumes/Projects/Software/bb-project/internal/state/store.go`
   - Change repo metadata load/save/path helpers to key by `repo_key`.
   - `LoadAllRepoMetadata` should ignore entries missing `repo_key`.
   - Sort metadata list by `repo_key`.

4. **Refactor scan/init metadata lifecycle**
   - File: `/Volumes/Projects/Software/bb-project/internal/app/app.go`
   - `discoveredRepo` gains `RepoKey`.
   - Discovery only emits repos matching strict depth.
   - `ensureRepoMetadata` accepts/uses `repo_key`.
   - Machine records write `repo_key`.
   - `resolveInitTarget`:
     - Explicit project must match catalog depth exactly.
     - CWD inference uses first N segments (N=depth), then repo root path is fixed there.

5. **Refactor sync reconcile to repo_key**
   - File: `/Volumes/Projects/Software/bb-project/internal/app/sync_phase_reconcile.go`
   - Replace all repo-id grouping/matching with repo-key grouping/matching.
   - Remove duplicate-local-repo-id unsyncable branch.
   - Parse `repo_key` to select catalog and relative target path.
   - Keep origin mismatch checks against `repo_id` for safety on target-path adoption.

6. **Refactor sync observe/publish and notification keys**
   - Files:
     - `/Volumes/Projects/Software/bb-project/internal/app/sync_phase_observe.go`
     - `/Volumes/Projects/Software/bb-project/internal/app/sync_phase_publish.go`
     - `/Volumes/Projects/Software/bb-project/internal/app/sync_phase_notify.go`
   - Metadata lookup by `repo_key`.
   - Previous-record continuity keys use `repo_key|path`.
   - Notify cache key priority becomes: `repo_key`, then `repo_id`, then path/name fallback.

7. **Refactor fix/repo command metadata selection**
   - Files:
     - `/Volumes/Projects/Software/bb-project/internal/app/fix.go`
     - `/Volumes/Projects/Software/bb-project/internal/app/app.go` (`RunRepoPolicy`, `RunRepoPreferredRemote`)
   - Meta map keyed by `repo_key`.
   - `enable-auto-push` loads by `repo_key`.
   - Selector matching supports `repo_key`, `repo_id`, and `name`.
   - Ambiguous selector returns explicit error (especially for repeated `repo_id`).

8. **Update `bb config` interactive catalog management**
   - File: `/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`
   - Catalog table adds a layout/depth column.
   - Add catalog action button: `Toggle Layout` (1-level <-> 2-level).
   - New catalogs default to depth `1`.
   - Update button indexing/navigation and policy summary text accordingly.
   - Catalog add/edit flows remain otherwise unchanged.

9. **Docs update**
   - Files:
     - `/Volumes/Projects/Software/bb-project/README.md`
     - `/Volumes/Projects/Software/bb-project/docs/implemented/001-SPEC.md`
     - `/Volumes/Projects/Software/bb-project/docs/implemented/003-CONFIG_COMMAND_SPEC.md`
   - Document:
     - `repo_key` identity model.
     - New filename pattern.
     - Strict `repo_path_depth`.
     - Removal of duplicate-by-repo-id unsyncable rule.

### TDD Plan (Run in small loops)

1. **Domain tests first**
   - New tests for depth/key helpers (depth 1, depth 2, invalid shapes, parsing).
2. **State tests**
   - Filename generation from `repo_key` (catalog-prefixed examples).
   - `LoadAllRepoMetadata` skips old entries missing `repo_key`.
3. **App/unit tests**
   - `ensureRepoMetadata` keyed by `repo_key`.
   - `resolveFixTarget`/repo selector ambiguity with duplicate `repo_id`.
4. **Wizard tests**
   - Catalog layout column visible.
   - Toggle layout action changes depth and persists in model.
5. **E2E tests**
   - Existing duplicate-origin case (`/internal/e2e/path_test.go`) updated to verify duplicates are safe.
   - New depth-2 convergence case (`owner/repo` shape on both machines).
   - Depth mismatch case (repo at wrong depth is ignored).
   - Init depth validation case (`bb init` rejects invalid project depth).
6. **Full suite**
   - `go test ./...`

### Assumptions and Defaults Locked

1. **Chosen by you:** metadata filenames include **catalog prefix**.
2. **Chosen by you:** depth behavior is **strict exactly depth**.
3. Catalog default `repo_path_depth` is `1`.
4. No migration: existing repo metadata files without `repo_key` are ignored and left on disk.
5. `bb catalog add` CLI stays unchanged (new catalogs default depth 1); depth changes are done via `bb config` wizard.
