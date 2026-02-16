# Spec: `bb clone` and `bb link` with Catalog-Aware Defaults, Sparse Clone, and Immediate State Registration

## Summary

Add two new top-level commands:

1. `bb clone` to clone/register repositories into a selected catalog with configurable defaults (global + per-catalog overrides).
2. `bb link` to create symlinks to local projects/repositories, optionally auto-cloning when the selector is a repo input and no local match exists.

Implementation is TDD-first (red tests, then implementation), with immediate metadata/machine-state updates for affected repo(s) and no full-scan requirement post-command.

## Motivation

1. Current onboarding covers `bb init` (create/adopt local project), but not “bring an existing remote repo into catalog-managed state”.
2. Referencing repos inside other repos is common (especially under `references/`), and manual symlink creation is repetitive and error-prone.
3. Users need clone behavior defaults that match workflow intent (for example shallow + partial clone for references).
4. Command results should be reflected in `bb` state immediately so follow-up commands (`status`, `fix`, `repo`) are deterministic.

## Public CLI/API Changes

1. Add command:

```bash
bb clone <repo> [--catalog <catalog>] [--as <relative-path>] [--shallow|--no-shallow] [--filter <spec>|--no-filter] [--only <path>...]
```

2. Add command:

```bash
bb link <project-or-repo> [--as <link-name>] [--dir <target-dir>] [--absolute] [--catalog <catalog>]
```

3. Add app runner methods in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go` and app methods in `/Volumes/Projects/Software/bb-project/internal/app`:

- `RunClone(opts CloneOptions) (int, error)`
- `RunLink(opts LinkOptions) (int, error)`

4. Add git runner method in `/Volumes/Projects/Software/bb-project/internal/gitx/git.go`:

- `CloneWithOptions(opts CloneOptions) error`
- Keep existing `Clone(origin, path)` as a thin wrapper for compatibility in internal call sites.

## Config and Schema Changes

Add to `ConfigFile` in `/Volumes/Projects/Software/bb-project/internal/domain/types.go`:

```yaml
clone:
  default_catalog: ""
  shallow: false
  filter: ""
  presets:
    references:
      shallow: true
      filter: blob:none
  catalog_preset:
    references: references
link:
  target_dir: references
  absolute: false
```

Go types:

1. `CloneConfig`:

- `DefaultCatalog string`
- `Shallow bool`
- `Filter string`
- `Presets map[string]ClonePreset`
- `CatalogPreset map[string]string`

2. `ClonePreset`:

- `Shallow *bool`
- `Filter *string`

3. `LinkConfig`:

- `TargetDir string`
- `Absolute bool`

Defaults:

1. `clone.default_catalog = ""`
2. `clone.shallow = false`
3. `clone.filter = ""`
4. `clone.presets.references = { shallow: true, filter: "blob:none" }`
5. `clone.catalog_preset.references = "references"` (explicit config mapping; not implicit by catalog name)
6. `link.target_dir = "references"`
7. `link.absolute = false`

Validation updates in `/Volumes/Projects/Software/bb-project/internal/app/config.go`:

1. `clone.default_catalog` may be empty.
2. `clone.filter` may be empty; non-empty accepts any git filter spec string.
3. `link.target_dir` must be non-empty and must not contain `..` path traversal.
4. Keep existing validations unchanged.

Wizard updates in `/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`:

1. Add editable fields for `clone.default_catalog`, `clone.shallow`, `clone.filter`.
2. Add editable clone preset rows and catalog->preset mapping rows.
3. Add editable fields for `link.target_dir`, `link.absolute`.

## Functional Spec: `bb clone`

1. Input `<repo>` accepted forms:

- GitHub shorthand `org/repo`
- GitHub HTTP/HTTPS repository link (for example `https://github.com/org/repo` or `http://github.com/org/repo`)
- HTTPS/SSH git URL
- SCP-like SSH remote (`git@host:path.git`)

2. Catalog selection precedence:

- `--catalog`
- `config.clone.default_catalog`
- error: `clone catalog is not configured; set clone.default_catalog or pass --catalog`

3. Chosen catalog must exist on current machine; if absent, error with friendly message naming catalog and suggesting `bb catalog list`.

4. Repo target path derivation:

- Uses chosen catalog `repo_path_depth`.
- Depth `1`: default relative path is `<repo-name>`.
- Depth `2`: default relative path is `<owner>/<repo>` only when derivable.
- If derivation not possible, require `--as`.
- `--as` is catalog-relative path and must match required depth exactly.

5. Existing local repo behavior:

- If same normalized origin already exists on this machine at any path, clone is no-op.
- Output notice includes actual repo name and local path.
- Ensure metadata/machine record exists (register if missing).

6. Path conflicts:

- If derived target path conflicts with existing non-matching project/repo, return error instructing to use `--as`.
- If `--as` path also conflicts, return hard error (no overwrite behavior).

7. Clone option resolution:

- Start with `config.clone.{shallow,filter}`.
- Resolve preset name from `config.clone.catalog_preset[<catalog>]` and apply `config.clone.presets[<preset>]` if found.
- Apply CLI overrides:
  - `--shallow` or `--no-shallow` (mutually exclusive)
  - `--filter <spec>` or `--no-filter` (mutually exclusive)
- `--only` is repeatable and enables sparse clone flow.
- Duplicate `--only` entries are deduped preserving first-seen order.

8. Sparse clone flow:

- Clone command includes `--sparse` when `--only` is non-empty.
- After clone, run `git sparse-checkout set --no-cone <paths...>`.

9. Immediate state updates:

- Create/backfill repo metadata immediately.
- Observe cloned repo and upsert machine repo record immediately.
- Save machine file immediately.
- No full scan required.

## Functional Spec: `bb link`

1. Anchor path:

- If current directory is inside a git repo, anchor is repo root (not current subdirectory).
- Else anchor is current directory.

2. Selector `<project-or-repo>` resolution order:

- Try local selector first:
  - exact repo_key
  - `catalog:project` form mapped to repo_key semantics
  - unique repo name
- If no local match and input parses as remote repo spec, auto-clone using `RunClone` internals.
- Auto-clone catalog selection for this path:
  - `bb link --catalog`
  - `config.clone.default_catalog`
  - error if unset

3. Link naming:

- `--as <link-name>` if set.
- Else use resolved project/repo name.

4. Target link directory:

- `--dir` if set; else `config.link.target_dir`.
- Relative values resolve against anchor.
- Absolute values are used as provided.

5. Symlink target mode:

- `--absolute` forces absolute symlink.
- Else use relative symlink by default (or config if `link.absolute=true` and no CLI override).

6. Idempotency and conflicts:

- Existing symlink with same resolved target is no-op.
- Existing file/dir/symlink-to-different-target is error.
- Never overwrite existing path.

## Internal Implementation Breakdown

1. CLI wiring in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`:

- Extend `appRunner` interface.
- Add `newCloneCommand` and `newLinkCommand`.
- Add root command registration.
- Flag validation and mutual exclusion checks.

2. App layer:

- Add `/Volumes/Projects/Software/bb-project/internal/app/clone.go`.
- Add `/Volumes/Projects/Software/bb-project/internal/app/link.go`.
- Add shared helpers in `/Volumes/Projects/Software/bb-project/internal/app`:
  - repo source parsing/normalization
  - clone option resolution from config + preset mapping + flags
  - catalog resolution for clone/link
  - repo record upsert helper
  - local repo lookup by normalized origin and selector

3. Git layer:

- Update `/Volumes/Projects/Software/bb-project/internal/gitx/git.go` with clone options support and sparse post-step.

4. State/domain:

- Extend config structs and defaults in:
  - `/Volumes/Projects/Software/bb-project/internal/domain/types.go`
  - `/Volumes/Projects/Software/bb-project/internal/state/store.go`
  - `/Volumes/Projects/Software/bb-project/internal/app/config.go`
  - `/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`

5. Docs:

- Update `/Volumes/Projects/Software/bb-project/README.md`.
- Regenerate CLI docs and man pages via `/Volumes/Projects/Software/bb-project/cmd/bb-docs`.
- Add spec document `/Volumes/Projects/Software/bb-project/docs/implemented/010-CLONE-LINK.md`.

## TDD Plan (Red -> Green)

1. **Domain/state red tests**

- Add config default tests in `/Volumes/Projects/Software/bb-project/internal/state/store_test.go`:
  - clone/link defaults exist.
  - references preset and catalog preset mapping defaults exist.
- Add config load/backfill tests:
  - missing clone/link blocks backfill properly.
- Add config validation tests in `/Volumes/Projects/Software/bb-project/internal/app/config_test.go`:
  - invalid `link.target_dir` rejected.

2. **CLI red tests**

- Extend `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`:
  - forwarding tests for clone/link options.
  - mutual exclusion tests (`--shallow` + `--no-shallow`, `--filter` + `--no-filter`).
  - argument count tests.
  - ensure commands appear in root help tree.

3. **Git red tests**

- Add tests in `/Volumes/Projects/Software/bb-project/internal/gitx/git_test.go`:
  - shallow clone sets shallow repository.
  - partial clone sets `remote.origin.partialclonefilter`.
  - sparse clone checks out only requested paths.

4. **App clone red tests**

- Add `/Volumes/Projects/Software/bb-project/internal/app/clone_test.go`:
  - catalog precedence and missing default clone catalog error.
  - missing selected catalog on machine error.
  - depth derivation and `--as` requirement.
  - conflict requires `--as`.
  - same-origin existing repo returns no-op with location notice.
  - immediate metadata + machine-record write without full scan.
  - `--only` triggers sparse flow and options merge rules.
  - github HTTP link input parses and clones.
  - preset mapping is only applied via `config.clone.catalog_preset`.

5. **App link red tests**

- Add `/Volumes/Projects/Software/bb-project/internal/app/link_test.go`:
  - anchor at repo root when inside git repo.
  - anchor at cwd when not in repo.
  - relative symlink default behavior.
  - `--absolute` behavior.
  - `--dir` override behavior.
  - conflict/no-op symlink semantics.
  - remote selector auto-clones when missing.
  - auto-clone fails when clone default catalog missing.
  - selector ambiguity errors.

6. **E2E red tests**

- Add `/Volumes/Projects/Software/bb-project/internal/e2e/clone_link_test.go`:
  - `bb clone org/repo` into configured clone default catalog.
  - `bb clone --catalog references` applies references preset by default.
  - `bb clone --only ...` sparse checks paths.
  - `bb link <existing-project>` creates relative link under repo root `references`.
  - `bb link <remote>` auto-clones then links.
  - existing same-origin at different path yields no-op + informative notice.
  - machine missing selected catalog errors friendly.

7. **Docs/tests red**

- Extend `/Volumes/Projects/Software/bb-project/cmd/bb-docs/main_test.go` to assert docs include `clone` and `link`.
- Update README command reference assertions if present.

8. **Green phase implementation order**

- Implement domain/state structs and defaults.
- Implement CLI parsing/forwarding.
- Implement git clone options.
- Implement app clone flow.
- Implement app link flow.
- Implement config wizard fields.
- Regenerate docs.
- Run full test suite and fix failures.

9. **Parallelization requirement**

- All new top-level tests and independent subtests must call `t.Parallel()`.

## Acceptance Criteria

1. `bb clone` supports all requested input forms and flags.
2. Clone destination catalog uses global clone default or explicit `--catalog`; no implicit fallback to machine default catalog.
3. Missing chosen catalog on machine returns clear actionable error.
4. `repo_path_depth` controls path derivation exactly as specified.
5. Non-derivable or conflicting target requires `--as`.
6. Existing same-origin local repo results in no-op with path/name notice.
7. `--only` performs sparse clone with requested paths.
8. Filter and shallow settings are configurable globally and per-catalog, with CLI overrides.
9. `bb link` anchors to repo root when inside git repo.
10. `bb link` creates idempotent symlinks with conflict safety.
11. `bb link` auto-clones remote input when needed, using clone catalog resolution rules.
12. Clone/link updates repo metadata and machine record immediately for affected repo.

## Explicit Assumptions and Defaults

1. `--as` semantics:

- `bb clone --as` is catalog-relative project path.
- `bb link --as` is link file/dir name.

2. Default clone catalog is global (`config.clone.default_catalog`) and is required when `--catalog` is omitted.
3. If default clone catalog name exists in config but not on current machine, command errors with friendly guidance.
4. Per-catalog preset model is config-driven; shipped default includes `references => shallow=true, filter=blob:none`.
7. Presets are never auto-applied by catalog name unless `clone.catalog_preset` maps that catalog to a preset.
5. Existing same-origin repo on machine is a no-op, not an error, and not duplicated.
6. No destructive behavior (`--force`, overwrite) is included in this scope.
