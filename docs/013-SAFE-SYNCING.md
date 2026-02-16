# Spec/Plan: Safe Catalog-Scoped Syncing

## Summary

This change hardens multi-machine syncing around one rule: **a repo only syncs into the catalog encoded in its `repo_key`**.  
No cross-catalog fallback is allowed.

It also changes clone behavior:

- `bb sync` does **not** clone missing repos by default.
- cloning during sync is opt-in per catalog via `auto_clone_on_sync`.
- missing local clones are surfaced as `clone_required` and fixed via `bb fix` (`clone` action, including CLI form).

Onboarding via `bb config` is improved:

- catalogs known from other machines are shown as remote-only rows
- selecting one and pressing Add opens a prefilled editor (name + suggested roots)
- users can cycle root suggestions from other machines

## Motivation

The previous fallback behavior could mix catalogs across machines (for example, metadata from catalog `A` ending up cloned into catalog `B` on a new machine).  
That creates long-term identity drift, path conflicts, and ambiguous ownership of repo locations.

Separately, implicit cloning during sync can be too aggressive for first-time onboarding and large catalogs.

## Goals

- enforce strict catalog identity for convergence
- prevent cross-catalog cloning/migration
- make sync cloning explicit and policy-controlled per catalog
- expose uncloned-but-known repos in `bb fix` with a direct clone fix path
- make onboarding discoverable by surfacing remote-known catalogs in `bb config`

## Non-Goals / Accepted Decisions

- `bb link` behavior remains unchanged
- no migration path is provided (no users yet)
- include-catalog mismatch may fail, but should provide remote-known guidance when possible

## Behavior Specification

### 1. Strict catalog matching in sync

For each repo metadata entry:

1. parse `repo_key` into `<catalog>/<relative-path>`
2. resolve target catalog by exact catalog name from `repo_key`
3. if that catalog is not locally configured, skip reconcile for this repo and warn
4. do not fallback to preferred/default/any other catalog

This guarantees no catalog mixing.

### 2. Clone policy: opt-in per catalog

New catalog field:

```yaml
catalogs:
  - name: software
    root: /Volumes/Projects/Software
    auto_clone_on_sync: false
```

Default is `false`.

When sync determines a local copy is missing/empty:

- if `auto_clone_on_sync=true`: clone + branch ensure + pull flow proceeds
- if `auto_clone_on_sync=false`: no clone; repo is marked unsyncable with `clone_required`

### 3. Exit semantics and notifications

`clone_required` (and `catalog_not_mapped`) are non-blocking reasons:

- `bb sync` exit code `1` only for blocking unsyncable reasons
- notify path ignores repos that have only non-blocking unsyncable reasons

This keeps “not cloned yet” visible without failing healthy sync convergence checks.

### 4. `--include-catalog` remote-known hinting

If `--include-catalog <name>` references a catalog that is not local but is known from other machines, sync returns selection error with guidance to run `bb config` and map it locally.

### 5. `bb config` onboarding UX

Wizard input now includes remote-known catalog roots collected from other machine files.

Catalog step behavior:

- local catalogs render normally
- remote-known missing catalogs render as read-only rows (`remote-only`)
- selecting remote-only row + Add opens Add editor prefilled with:
  - catalog name
  - best suggested root
- root suggestions can be cycled in the editor
- catalog edit now includes `Auto clone during sync` toggle (`auto_clone_on_sync`)

### 6. `bb fix` clone workflow

`bb fix` augments local snapshot with synthetic `clone_required` rows for known metadata entries when:

- catalog is selected and locally mapped
- repo metadata exists
- no local match exists
- target path is safe
- catalog clone policy is disabled

Eligible actions now include:

- `clone` when reason includes `clone_required`

CLI equivalent:

- `bb fix <repo_key|name|path> clone`

Clone action flow:

1. validate metadata/path/repo key
2. pick sync winner from machine records
3. validate target path safety
4. create parent dir (if needed), clone if needed
5. verify/adopt matching origin
6. ensure winner branch
7. optional fetch-prune
8. pull ff-only
9. refresh status

## Data Model Changes

- `Catalog.AutoCloneOnSync *bool` (`yaml:"auto_clone_on_sync,omitempty"`)
- new unsyncable reasons:
  - `clone_required`
  - `catalog_not_mapped`
- helper classification:
  - `IsBlockingUnsyncableReason`
  - `HasBlockingUnsyncableReason`

## TDD Plan and Validation

Implemented with red-first tests, then incremental code changes.

Coverage includes:

- strict catalog reconcile behavior (`internal/e2e/catalog_test.go`)
- no implicit clone by default + clone-required reporting (`internal/e2e/path_test.go`)
- `bb fix ... clone` CLI flow (`internal/e2e/path_test.go`)
- sync non-blocking exit semantics (`internal/app/sync_orchestrator_test.go`)
- config onboarding known-catalog discovery and prefill (`internal/app/config_test.go`, `internal/app/config_wizard_test.go`)
- catalog wizard auto-clone policy editing (`internal/app/config_wizard_test.go`)

Validation run:

- `go test ./internal/domain ./internal/app ./internal/e2e -count=1`

## Risks / Notes

- repos in remote-only catalogs will not converge locally until catalog mapping is added in `bb config`
- enabling `auto_clone_on_sync` on large catalogs can still trigger substantial clone activity; keep default disabled unless explicitly desired
