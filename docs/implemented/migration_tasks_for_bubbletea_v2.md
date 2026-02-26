# Bubble Tea v2 Migration Tasks (Repo Checklist)

## Sources of truth

- Upgrade guide: `references/vendor/bubbletea/UPGRADE_GUIDE_V2.md`
- Bubble Tea v2 API reference in repo: `references/vendor/bubbletea/tea.go`, `references/vendor/bubbletea/key.go`
- Local reference modules (v2 path examples):
  - `references/vendor/bubbletea/go.mod` (`module charm.land/bubbletea/v2`)
  - `references/vendor/bubbles/go.mod` (`module charm.land/bubbles/v2`)
  - `references/vendor/lipgloss/go.mod` (`module charm.land/lipgloss/v2`)

## Audit snapshot (current state)

- Direct Bubble Tea imports still v1 path in 6 files:
  - `internal/app/config_wizard.go`
  - `internal/app/fix_tui.go`
  - `internal/app/fix_tui_wizard.go`
  - `internal/app/config_wizard_test.go`
  - `internal/app/fix_tui_test.go`
  - `internal/app/fix_tui_stage_stash_summary_test.go`
- Direct Lip Gloss imports still v1 path in 5 files:
  - `internal/app/config_wizard.go`
  - `internal/app/fix_tui.go`
  - `internal/app/fix_tui_wizard.go`
  - `internal/app/fix_tui_test.go`
  - `internal/app/ui_badge.go`
- Direct Bubbles imports still pre-v2 path in 4 files:
  - `internal/app/config_wizard.go`
  - `internal/app/fix_tui.go`
  - `internal/app/fix_tui_wizard.go`
  - `internal/app/fix_tui_test.go`
- `View() string` models found:
  - `internal/app/config_wizard.go` (`configWizardModel.View`)
  - `internal/app/fix_tui.go` (`fixTUIBootModel.View`, `fixTUIModel.View`)
- `WithAltScreen` usage found:
  - `internal/app/config_wizard.go` (`tea.NewProgram(... tea.WithAltScreen(), tea.WithFilter(...))`)
  - `internal/app/fix_tui.go` (`tea.NewProgram(... tea.WithAltScreen())`)
- Key handling still v1 patterns:
  - Runtime key handlers use `tea.KeyMsg` and type assertions to `tea.KeyMsg`
  - 10 runtime `" "` comparisons that must become `"space"`
  - 128 test key literals using `tea.KeyMsg{Type: ..., Runes: ..., Alt: ...}`
- No direct usage found for removed/renamed mouse APIs, removed program methods, `tea.Sequentially`, `tea.WindowSize()`, or paste-old patterns.

## Full guide checklist mapped to this repo

Guide checklist items from `UPGRADE_GUIDE_V2.md` mapped to concrete repo actions.

1. Update import paths ([Import Paths](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#import-paths))
- Required.
- Update module deps + imports from GitHub paths to `charm.land/.../v2`.

2. Change `View() string` to `View() tea.View` ([View Returns a `tea.View` Now](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#view-returns-a-teaview-now))
- Required.
- 3 model methods must migrate.

3. Replace `tea.KeyMsg` with `tea.KeyPressMsg` ([Key Messages](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#key-messages))
- Required.
- Runtime switches/assertions and helper signatures must change.

4. Update key fields (`msg.Type` / `msg.Runes` / `msg.Alt`) ([Key Messages](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#key-messages))
- Required in tests.
- Runtime mostly uses `String()` and `key.Matches`; tests use legacy struct fields heavily.

5. Replace `case " ":` with `case "space":` ([Key Messages](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#key-messages))
- Required.
- `internal/app/config_wizard.go`, `internal/app/fix_tui_wizard.go`.

6. Update mouse message usage ([Mouse Messages](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#mouse-messages))
- Not currently applicable (no mouse events in owned app code).
- Keep grep verification in migration gate.

7. Rename mouse button constants ([Mouse Messages](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#mouse-messages))
- Not currently applicable.

8. Remove old program options and use View fields ([Removed Program Options](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#removed-program-options))
- Required for `WithAltScreen`.
- `WithFilter` is still used and should be re-validated against v2 API during compile/test.

9. Remove old commands and use View fields ([Removed Commands](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#removed-commands))
- Not currently applicable in repo code (no `tea.EnterAltScreen`, etc.).

10. Remove old program methods ([Removed Program Methods](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#removed-program-methods))
- Not currently applicable (already uses `Run()`).

11. Rename `tea.WindowSize()` to `tea.RequestWindowSize` ([Renamed APIs](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#renamed-apis))
- Not currently applicable (no `tea.WindowSize()` calls).

12. Replace `tea.Sequentially(...)` with `tea.Sequence(...)` ([Renamed APIs](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#renamed-apis))
- Not currently applicable.

## Migration work plan (ordered, actionable)

## Phase 1: Dependency and import-path migration

Goal: get the repo onto v2 module paths first, then resolve compile errors deliberately.

Tasks:
- Update `go.mod` requirements:
  - `github.com/charmbracelet/bubbletea` -> `charm.land/bubbletea/v2`
  - `github.com/charmbracelet/lipgloss` -> `charm.land/lipgloss/v2`
  - `github.com/charmbracelet/bubbles` -> `charm.land/bubbles/v2`
- Run `go mod tidy` to refresh `go.sum`.
- Update imports in:
  - `internal/app/config_wizard.go`
  - `internal/app/fix_tui.go`
  - `internal/app/fix_tui_wizard.go`
  - `internal/app/ui_badge.go`
  - `internal/app/config_wizard_test.go`
  - `internal/app/fix_tui_test.go`
  - `internal/app/fix_tui_stage_stash_summary_test.go`

Dependencies:
- Must be first; later API migrations depend on v2 types (`tea.View`, `tea.KeyPressMsg`, `tea.Key` fields).

## Phase 2: Model/View contract and alt-screen migration

Guide refs:
- [View Returns a `tea.View` Now](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#view-returns-a-teaview-now)
- [Removed Program Options](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#removed-program-options)
- [The Big Idea: Declarative Views](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#the-big-idea-declarative-views)

Tasks:
- Convert model view methods:
  - `internal/app/config_wizard.go`: `func (m *configWizardModel) View() tea.View`
  - `internal/app/fix_tui.go`: `func (m *fixTUIBootModel) View() tea.View`
  - `internal/app/fix_tui.go`: `func (m *fixTUIModel) View() tea.View`
- Wrap existing rendered string content with `tea.NewView(content)`.
- Move alt-screen behavior from program options to view fields:
  - Remove `tea.WithAltScreen()` from `tea.NewProgram(...)` in:
    - `internal/app/config_wizard.go`
    - `internal/app/fix_tui.go`
  - Set `v.AltScreen = true` in migrated view methods where full-screen UX is expected.
- Keep and re-test filter behavior in config wizard:
  - `internal/app/config_wizard.go`: `tea.WithFilter(configWizardFilter)` (confirm option still valid in v2 during compile/test).

Local examples to update:
- `internal/app/config_wizard.go` (`runConfigWizardInteractive`, `View`)
- `internal/app/fix_tui.go` (`runFixInteractive`, `fixTUIBootModel.View`, `fixTUIModel.View`)

Dependencies:
- Do after Phase 1 import migration.

## Phase 3: Key-event migration (runtime)

Guide refs:
- [Key Messages](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#key-messages)
- [Space Bar Changed](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#space-bar-changed)
- [Ctrl+key Matching](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#ctrlkey-matching)

Tasks:
- Runtime message switches:
  - Replace `case tea.KeyMsg:` with `case tea.KeyPressMsg:` in:
    - `internal/app/config_wizard.go`
    - `internal/app/fix_tui.go`
- Runtime type assertions:
  - Replace `msg.(tea.KeyMsg)` with `msg.(tea.KeyPressMsg)` in:
    - `internal/app/config_wizard.go` (`updateIntro`, `updateGitHub`, `updateSync`, `updateAutomation`, `updateFixes`, `updateCatalogs`, `updateCatalogEditor`, `updateReview`)
- Helper signatures:
  - `internal/app/fix_tui_wizard.go`
    - `updateWizard(msg tea.KeyMsg)` -> `updateWizard(msg tea.KeyPressMsg)`
    - `updateSummary(msg tea.KeyMsg)` -> `updateSummary(msg tea.KeyPressMsg)`
    - `isWizardVisualDiffShortcut(msg tea.KeyMsg)` -> `isWizardVisualDiffShortcut(msg tea.KeyPressMsg)`
- Space-key string deltas (`" "` -> `"space"`) in runtime logic:
  - `internal/app/config_wizard.go` (all `case " ":` in update handlers)
  - `internal/app/fix_tui_wizard.go` (`case " ":` and `== " "` checks)
- Keep `ctrl+c` and `alt+v` handling, but verify against v2 string representation.

Risk notes:
- Using `tea.KeyMsg` interface in v2 can accidentally process key-release events. This repo should remain press-only; use `tea.KeyPressMsg` for deterministic behavior.

Dependencies:
- Requires Phase 2 to compile cleanly around updated signatures.

## Phase 4: Test migration (key construction + assertions)

Guide refs:
- [Key fields changed](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#key-fields-changed)
- [Space Bar Changed](../references/vendor/bubbletea/UPGRADE_GUIDE_V2.md#space-bar-changed)

Tasks:
- Replace v1 key-literal construction (`tea.KeyMsg{Type: ..., Runes: ..., Alt: ...}`) in:
  - `internal/app/config_wizard_test.go`
  - `internal/app/fix_tui_test.go`
  - `internal/app/fix_tui_stage_stash_summary_test.go`
- Migrate to v2 press messages using `tea.KeyPressMsg` with v2 fields (`Code`, `Text`, `Mod`).
- Update all tests that currently model space as runes/`KeySpace` so that code paths keyed on `msg.String()` use `"space"` semantics.
- Update alt-modified key tests (visual diff shortcut):
  - Existing `Alt: true` usage in `internal/app/fix_tui_test.go` must become `Mod: tea.ModAlt`.
- Recommended cleanup during migration:
  - Add local test helper(s) for key presses to avoid 128 manual ad hoc literals and reduce future churn.

Dependencies:
- Must follow runtime key migration so tests target final behavior.

## Phase 5: Non-code docs and local guidance alignment

Tasks:
- Update outdated example snippet using old options:
  - `docs/implemented/003-CONFIG_COMMAND_SPEC.md` (`tea.NewProgram(model, tea.WithAltScreen(), tea.WithFilter(...))`)
  - Replace with v2 declarative-view framing (`View().AltScreen = true`; keep filter mention if still valid).
- Update contributor guidance link pointing to old bubbletea import path:
  - `AGENTS.md` (`pkg.go.dev/github.com/charmbracelet/bubbletea`) -> v2 docs/import path wording.
- Run a doc grep to ensure no stale v1 import-path snippets remain in maintained docs:
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/lipgloss`

Dependencies:
- Can run in parallel with Phase 4 once migration direction is finalized.

## Phase 6: Final sweep for guide-completeness

Tasks:
- Confirm no stale APIs via grep:
  - `tea.KeyMsg` (runtime code should be press-specific except intentional interface usage)
  - `tea.WithAltScreen`
  - `case " "` in key-string switches
  - `tea.EnterAltScreen`, `tea.ExitAltScreen`, `tea.EnableMouse*`, `tea.DisableMouse`, `tea.ShowCursor`, `tea.HideCursor`
  - `tea.Sequentially`, `tea.WindowSize()`
  - v1 import paths for bubbletea/lipgloss/bubbles
- Confirm no accidental mouse/paste regressions:
  - No stale v1 mouse constants or paste flags introduced during refactor.

Dependencies:
- Last phase before release gate.

## Verification and release gate

Run after each phase where applicable; do full gate at the end.

1. Compile + targeted tests
- `go test ./internal/app -run 'ConfigWizard|FixTUI' -count=1`

2. Full tests
- `go test ./... -count=1`

3. Interactive smoke checks
- `bb config`
  - Verify full-screen behavior still active.
  - Verify unsaved-change quit filter still works.
  - Verify space toggles still function everywhere.
- `bb fix`
  - Verify boot screen and main screen alt-screen behavior.
  - Verify wizard/summary navigation, `space` toggles, `alt+v`, `ctrl+c`, help toggle.

4. Regression-specific checks
- Visual diff shortcut path still launches only on intended key combo.
- No double-trigger behavior from key-release events.
- View rendering unchanged semantically (same content, only return type changed).

## File-by-file task matrix

`go.mod`
- Replace Charm deps with v2 module paths (`charm.land/.../v2`).

`go.sum`
- Refresh via `go mod tidy`.

`internal/app/config_wizard.go`
- Import path migration.
- Remove `WithAltScreen` option.
- Convert `View() string` to `View() tea.View` and set `AltScreen = true`.
- Convert key handling to `tea.KeyPressMsg`.
- Replace all `" "` key-string matches with `"space"`.

`internal/app/fix_tui.go`
- Import path migration.
- Remove `WithAltScreen` option.
- Convert both `View() string` methods to `View() tea.View` and set `AltScreen = true`.
- Convert Update key switch to `tea.KeyPressMsg`.

`internal/app/fix_tui_wizard.go`
- Import path migration.
- Convert key function signatures to `tea.KeyPressMsg`.
- Replace all `" "` key-string checks with `"space"`.

`internal/app/ui_badge.go`
- Lipgloss import path migration only.

`internal/app/config_wizard_test.go`
- Bubble Tea import path migration.
- Replace legacy key literals with v2 press messages.

`internal/app/fix_tui_test.go`
- Bubble Tea/Lip Gloss/Bubbles import path migration.
- Replace legacy key literals (`Type`, `Runes`, `Alt`) with v2 fields (`Code`, `Text`, `Mod`).

`internal/app/fix_tui_stage_stash_summary_test.go`
- Bubble Tea import path migration.
- Replace legacy key literals with v2 press messages.

`docs/implemented/003-CONFIG_COMMAND_SPEC.md`
- Update outdated Bubble Tea setup examples (`WithAltScreen` guidance).

`AGENTS.md`
- Update old Bubble Tea documentation link/import references to v2.

## Local code examples to keep aligned while migrating

- Config wizard old patterns:
  - `internal/app/config_wizard.go` (`tea.NewProgram(... tea.WithAltScreen(), ...)`)
  - `internal/app/config_wizard.go` (`func (m *configWizardModel) View() string`)
  - `internal/app/config_wizard.go` (`case tea.KeyMsg`, `case " "`)
- Fix TUI old patterns:
  - `internal/app/fix_tui.go` (`tea.NewProgram(... tea.WithAltScreen())`)
  - `internal/app/fix_tui.go` (`func (m *fixTUIBootModel) View() string`, `func (m *fixTUIModel) View() string`)
  - `internal/app/fix_tui.go` (`case tea.KeyMsg`)
- Wizard key-string assumptions:
  - `internal/app/fix_tui_wizard.go` (`case " "`, `msg.String() == " "`, `isWizardVisualDiffShortcut(msg tea.KeyMsg)`)
- Test-construction old patterns:
  - `internal/app/config_wizard_test.go`, `internal/app/fix_tui_test.go`, `internal/app/fix_tui_stage_stash_summary_test.go` (`tea.KeyMsg{Type:..., Runes:..., Alt:...}`)
