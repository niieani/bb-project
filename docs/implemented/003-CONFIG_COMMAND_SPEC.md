## Interactive `bb config` Wizard (Onboarding + Reconfiguration)

### Summary

Implement a new `bb config` interactive command that:

- Uses Bubble Tea + Bubbles in full-screen mode for onboarding and rerun editing.
- Edits all keys in `config.yaml` plus this machine file's `catalogs` and `default_catalog`.
- Applies changes only after review/confirm.
- Does not auto-run `scan`/`status` after save.
- Enforces non-empty `github.owner` (breaking change from current fallback behavior).

### Scope

- Add one new top-level command: `bb config`.
- Add interactive UX for:
  - Shared config `~/.config/bb-project/config.yaml`.
  - Current machine config `~/.config/bb-project/machines/<machine-id>.yaml`.
- Keep existing `bb catalog ...` commands unchanged and compatible.

### Non-goals

- No non-interactive mode/flags in v1.
- No automatic migration of `repos/*.yaml` `preferred_catalog` values.
- No automatic follow-up command execution after save.

## Vendor Clues Driving Design

The repo includes symlinked vendor references at:

- `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea`
- `/Volumes/Projects/Software/bb-project/references/vendor/bubbles`

These concrete patterns should be followed:

- Multi-field validated form pattern from `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/examples/credit-card-form/main.go`.
- Key map + mini/full help pattern from `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/examples/help/main.go`.
- Table focus/blur/resizing pattern from `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/examples/table/main.go` and `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/table/table.go`.
- Multi-view step routing from `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/examples/views/main.go`.
- Returning final model result after `Run()` from `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/examples/result/main.go`.
- Program options for altscreen/filter/custom I/O from `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/options.go`.

## Important API / Interface / Type Changes

- CLI surface:
  - Add `config` dispatch in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`.
  - Update usage output in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`.
- App layer:
  - Add `RunConfig()` in `/Volumes/Projects/Software/bb-project/internal/app/config.go`.
  - Add UI runner abstraction for testability without real TTY.
  - Add apply-time conflict detection before writing files.
- New UI package:
  - `/Volumes/Projects/Software/bb-project/internal/ui/configwizard`.
  - One parent Bubble Tea model with step-specific subviews.
- State helpers:
  - Add explicit `SaveConfig(paths, cfg)` in `/Volumes/Projects/Software/bb-project/internal/state/store.go`.
  - Add helper to load machine/config for config flow without `loadContext()` side effects.
- Validation behavior update (breaking):
  - `github.owner` must be non-empty and trimmed.
  - Remove `init` fallback owner `"you"`; commands that rely on owner should fail with explicit guidance until fixed.

## Bubble Tea / Bubbles Architecture (Decision Complete)

### Program setup

- Run with `tea.NewProgram(model, tea.WithAltScreen(), tea.WithFilter(...))`.
- Use `tea.WithInput` and `tea.WithOutput` in tests to simulate keyboard sequences and capture renders.
- Return final model from `Program.Run()` and apply filesystem writes in app layer only after explicit confirmation.

### Model composition

- Parent model fields:
  - `step` enum.
  - `dirty` flag.
  - `errors` by field.
  - `viewportWidth`, `viewportHeight`.
  - `help.Model` and custom key map.
  - Snapshot of original config/machine for diff and conflict detection.
- Child state:
  - `[]textinput.Model` for text fields.
  - Toggle/select controls for booleans/enums.
  - `table.Model` for catalogs.
  - Optional modal editor state for add/edit catalog root.

### Components

- `bubbles/textinput`:
  - Use `Validate` and `Err` on each field.
  - `github.owner` validator: trimmed, non-empty.
  - `notify.throttle_minutes` validator: integer and `>= 0`.
- `bubbles/table`:
  - Catalog listing with columns `Name`, `Root`, `Layout`, `Default`.
  - Uses focus/blur when entering/exiting catalog edit mode.
  - Dynamically resized via `SetWidth`/`SetHeight` on `tea.WindowSizeMsg`.
- `bubbles/key` + `bubbles/help`:
  - Central keymap and generated short/full help footer.
  - `?` toggles full help.

### View routing

- Parent `Update` routes to per-step update functions.
- Steps:
  1. Intro.
  2. GitHub + state transport.
  3. Sync options.
  4. Notify options.
  5. Catalog management.
  6. Review + apply.

## UX / Interaction Spec

### Entry behavior

- `bb config` with unknown args returns usage error (exit `2`).
- Requires interactive terminal; otherwise fail with `bb config requires an interactive terminal`.

### Flow details

1. Intro

- Detect onboarding vs reconfigure mode.
- Show paths that will be modified.

2. GitHub + transport

- `github.owner` required text input.
- `github.default_visibility` selection (`private|public`).
- `github.remote_protocol` selection (`ssh|https`).
- Show `state_transport.mode`, constrain to `external`.

3. Sync

- Edit all `sync.*` booleans.

4. Notify

- Edit `notify.enabled`, `notify.dedupe`, `notify.throttle_minutes`.

5. Catalogs

- List and manage catalogs:
  - Add catalog (`name`, `root`).
  - Edit existing catalog root.
  - Remove catalog (confirm).
  - Set default catalog.
  - Toggle repository path layout depth (`1` or `2` levels under catalog root).
- Catalog name immutable after creation.
- Missing root directories can be created during apply flow.

6. Review + apply

- Show semantic diff for both files.
- Show side effects (directories to create).
- Confirm apply/cancel.

### Key controls

- `Tab`/`Shift+Tab`: next/previous field.
- `n`/`p`: next/previous step (blocked when step invalid).
- `Enter`/`Space`: activate/select.
- `a` `e` `d` `s`: add/edit/delete/set-default in catalog step.
- Catalog action row includes a dedicated `Toggle Layout` action.
- `Ctrl+S`: apply from review step.
- `Esc`: back/cancel current modal.
- `Ctrl+C`: quit (blocked by unsaved-change filter unless discard confirmed).

## Validation Rules

- `github.owner` must be non-empty after trim.
- `notify.throttle_minutes` must be integer `>= 0`.
- At least one catalog required.
- `default_catalog` must reference an existing catalog.
- Catalog names unique and non-empty.
- Catalog roots absolute and non-empty.
- Catalog `repo_path_depth` must be `1` or `2` (or omitted/`0`, which defaults to `1`).
- Missing catalog roots:
  - Prompt to create during apply.
  - If user declines and root is still missing, block save.
- Unsupported loaded `state_transport.mode` shown as invalid input and forced to `external` before save.

## Breaking Change and Migration Notes

- Breaking change: blank `github.owner` is no longer allowed.
- Existing users with blank owner:
  - `bb config` highlights this immediately and blocks apply until fixed.
  - `bb init` should fail with explicit message instead of silently defaulting to `"you"`.
- Migration guidance should be printed in failing commands:
  - `run 'bb config' and set github.owner`.

## Persistence / Concurrency / Safety

- No writes until final confirmation.
- Acquire global lock only during apply.
- Before write:
  - Re-read current files.
  - Compare against the original load snapshot.
  - Abort if changed externally; instruct user to rerun.
- Persist order:
  1. `config.yaml`.
  2. machine file (preserve existing `repos` list).
- Set `machine.updated_at` when machine file is updated.
- Cancel path guarantees zero mutation.

## Implementation Breakdown

1. CLI wiring

- Add `config` command parse/dispatch.
- Update help usage text.

2. App orchestration

- Load baseline config/machine models.
- Run wizard program.
- Apply confirmed changes under lock with conflict detection.

3. UI package

- Parent model with per-step update/view handlers.
- Text input validators wired directly via `textinput.Validate`.
- Catalog table + modal editor.
- Keymap/help footer integration.
- Window resize support.

4. Save layer

- Save helpers.
- Directory creation on confirmed apply.
- Apply-time snapshot conflict checks.

5. Docs

- Update `/Volumes/Projects/Software/bb-project/README.md` command reference and first-time setup flow.

## TDD Test Plan

### Unit tests

- `/Volumes/Projects/Software/bb-project/internal/ui/configwizard/model_test.go`
  - `github.owner` required validation.
  - Field-level validation and step blocking.
  - Catalog add/edit/remove/default and name immutability.
  - Dirty-state and quit-filter behavior.
  - Window-size resize behavior for table/form layout.
- `/Volumes/Projects/Software/bb-project/internal/app/config_test.go`
  - Confirm apply writes both files.
  - Cancel produces no writes.
  - Apply conflict detection aborts safely.
  - Missing catalog root create/decline paths.
- `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`
  - `config` dispatch and usage errors.

### Integration tests

- `/Volumes/Projects/Software/bb-project/internal/e2e/config_interactive_test.go`
  - Non-TTY failure path.
  - Onboarding happy path via fake UI runner.
  - Rerun reconfiguration updates existing values.
  - Blank owner causes validation failure until corrected.

### Acceptance scenarios

- First-run onboarding completes without editing files manually.
- Rerun supports safe, interactive edits of existing values.
- Blank `github.owner` is blocked everywhere relevant with clear remediation.
- Catalog path creation prompt works as specified.
- Save remains race-safe.

## Assumptions and Defaults

- Command name is `bb config`.
- Full-screen wizard UX with reusable editing flow.
- Editable coverage includes all current `config.yaml` keys and machine catalogs/default.
- Catalog names are immutable after creation.
- No auto `scan`/`status` post-save.
