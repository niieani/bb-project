## Interactive `bb config` Wizard (Onboarding + Reconfiguration)

### Summary

Implement a new `bb config` interactive command that:

- Uses Bubble Tea + Bubbles for a full-screen setup wizard.
- Works for first-time onboarding and reruns for editing existing values.
- Edits all current `config.yaml` keys plus this machine’s catalogs/default catalog.
- Applies changes only after a final review confirmation.
- Does not auto-run `scan`/`status` after save.

### Scope

- Add one new top-level command: `bb config`.
- Add interactive UX for:
  - Shared config (`~/.config/bb-project/config.yaml`)
  - Current machine catalog config (`~/.config/bb-project/machines/<machine-id>.yaml`)
- Keep existing `bb catalog ...` commands unchanged and compatible.

### Non-goals

- No non-interactive flags in v1 of this command.
- No automatic migration of repo metadata for catalog renames.
- No automatic follow-up commands after save.

## Important API / Interface / Type Changes

- CLI surface:
  - Add `config` command dispatch in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`.
  - Update usage output in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go` and docs.
- App layer:
  - Add `RunConfig()` in `/Volumes/Projects/Software/bb-project/internal/app` (new file recommended: `/Volumes/Projects/Software/bb-project/internal/app/config.go`).
  - Add a UI runner abstraction in app package so behavior is testable without TTY.
- New UI package:
  - `/Volumes/Projects/Software/bb-project/internal/ui/configwizard`
  - Bubble Tea model + Bubbles components.
- State helpers (if needed for clarity/testability):
  - `SaveConfig(paths, cfg)` in `/Volumes/Projects/Software/bb-project/internal/state/store.go` (thin wrapper over `SaveYAML`).
  - Optional helper to load/bootstrap machine config without going through `loadContext()` constraints.

## UX / Interaction Spec (Decision Complete)

### Entry behavior

- `bb config` with any unknown arg returns usage error (exit `2`).
- Requires interactive terminal (`stdin` + `stdout` TTY). If not TTY, return explicit error (exit `2`): `bb config requires an interactive terminal`.

### Flow steps

1. Intro

- Detect mode:
  - Onboarding mode if no catalogs or obvious first-run defaults.
  - Reconfigure mode otherwise.
- Show file paths that will be edited.

2. GitHub settings

- `github.owner` (text input)
- `github.default_visibility` (`private|public`)
- `github.remote_protocol` (`ssh|https`)
- `state_transport.mode` displayed and constrained to `external` only.

3. Sync settings

- `sync.auto_discover`
- `sync.include_untracked_as_dirty`
- `sync.default_auto_push_private`
- `sync.default_auto_push_public`
- `sync.fetch_prune`
- `sync.pull_ff_only`

4. Notify settings

- `notify.enabled`
- `notify.dedupe`
- `notify.throttle_minutes` (integer)

5. Catalog management (machine file)

- Table of catalogs with default marker.
- Actions:
  - Add catalog (name + root)
  - Edit catalog root
  - Remove catalog (with confirmation)
  - Set default catalog
- Catalog name is immutable once created.

6. Review + apply

- Show a compact diff (before vs after).
- Show planned filesystem side effects (missing catalog root directories to create).
- Confirm apply or cancel.

### Key controls (help footer)

- `Tab`/`Shift+Tab`: move focus
- `Enter`/`Space`: select/toggle
- `n`/`p`: next/previous step
- `a` `e` `d` `s`: catalog actions
- `Ctrl+S`: apply
- `Ctrl+C`/`Esc`: cancel/back

## Validation Rules

- `notify.throttle_minutes >= 0`
- At least one catalog required to save.
- `default_catalog` must exist in catalogs.
- Catalog names must be unique and non-empty.
- Catalog roots must be absolute, non-empty paths.
- Missing catalog root paths:
  - User is prompted to create directories during apply.
  - If declined, save is blocked until valid roots exist.
- `github.owner` may be NOT blank - we'll need to update current validation
- Unsupported loaded `state_transport.mode` is surfaced and normalized to `external` before apply.

## Persistence / Concurrency / Safety

- No writes until final confirm.
- Acquire global lock only at apply time (not for entire wizard session).
- On apply:
  - Re-read target files and detect concurrent external edits since wizard opened.
  - If changed, abort apply with conflict message and ask user to rerun.
- Persist order:
  1. `config.yaml`
  2. machine file (catalog/default updates; preserve existing `repos` list)
- Set machine `updated_at` on successful save.
- Cancel path guarantees zero file mutation.

## Implementation Breakdown

1. CLI wiring

- Add `config` case and parser (no flags initially).
- Update usage string to include `config`.

2. App command

- Add `RunConfig()` orchestration in app package.
- Load config/machine without `loadContext()` hard-failing on unsupported mode.
- Invoke UI runner and handle result (`applied|cancelled|error`).

3. Bubble Tea UI package

- Build model, step router, validation, catalog table/actions, review screen.
- Use `bubbles/textinput`, `bubbles/list` or `bubbles/table`, `bubbles/help`, `bubbles/key`.

4. Save layer

- Add explicit save helpers and conflict detection.
- Implement directory creation prompts/results.

5. Docs

- Update `/Volumes/Projects/Software/bb-project/README.md`:
  - First-time setup should start with `bb config`.
  - Add command reference for `bb config`.
  - Keep existing `bb catalog`/manual flow as alternative.

## TDD Test Plan

### Unit tests

- `/Volumes/Projects/Software/bb-project/internal/ui/configwizard/model_test.go`
  - Step validation gates.
  - Catalog add/edit/remove/default behavior.
  - Name immutability.
  - Missing-root creation decision handling.
- `/Volumes/Projects/Software/bb-project/internal/app/config_test.go`
  - Applies edits to both files.
  - Cancel makes no changes.
  - Conflict-at-save aborts safely.
  - Unsupported loaded mode can be corrected and saved.
- `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`
  - `config` dispatch parsing and unknown-arg handling.

### E2E / integration-style tests

- `/Volumes/Projects/Software/bb-project/internal/e2e/config_interactive_test.go`
  - Non-TTY invocation fails with clear message.
- App-level integration with fake UI runner:
  - First-run onboarding writes valid config + machine catalogs.
  - Rerun updates existing values without manual edits.

### Acceptance scenarios

- First-time user can configure all required values and complete onboarding via `bb config`.
- Existing user can rerun `bb config` and change values interactively.
- Command blocks invalid saves with clear inline validation.
- Cancel exits cleanly with no mutations.
- Save is race-safe with apply-time lock and conflict detection.

## Assumptions and Defaults Chosen

- Command name is `bb config`.
- UX is a setup wizard plus reusable editable form with validation.
- Editable coverage includes all current `config.yaml` keys.
- Catalog operations include add/edit-root/remove/set-default.
- Catalog names are immutable.
- Missing catalog roots are creatable from wizard flow.
- No automatic `scan`/`status` after save; only suggested next commands are shown.
- Non-interactive mode is out of scope for this version.
