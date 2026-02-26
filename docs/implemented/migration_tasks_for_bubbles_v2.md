# Bubbles v2 Migration Tasks (`bb-project`)

Source of truth:
- `references/vendor/bubbles/UPGRADE_GUIDE_V2.md`

Audit scope completed:
- imports
- component API usage
- key map/key event handling
- tests
- docs

---

## 1. Inventory: current Bubbles-touching files

Code + tests:
- `go.mod`
- `internal/app/config_wizard.go`
- `internal/app/fix_tui.go`
- `internal/app/fix_tui_wizard.go`
- `internal/app/fix_tui_test.go`
- `internal/app/config_wizard_test.go`
- `internal/app/fix_tui_stage_stash_summary_test.go`
- `internal/app/ui_badge.go` (Lip Gloss breakage coupled to Bubbles v2 guide §2d/§4)

Docs:
- `docs/CLI_UX_AND_STYLE_GUIDE.md`
- `docs/implemented/003-CONFIG_COMMAND_SPEC.md`
- `docs/implemented/006-SWITCH_TO_LIST.md`
- `AGENTS.md`

Guide component coverage used in repo:
- `help`, `key`, `list`, `spinner`, `table`, `textinput`, `viewport`

Guide sections not currently used by repo code:
- `cursor`, `filepicker`, `paginator`, `progress`, `stopwatch`, `textarea`, `timer`

---

## 2. Ordered migration plan (complete path, no shortcuts)

## Phase A: Module + import path migration (blocker for all later phases)

Guide refs:
- §1 Import Paths
- §2a (`tea.KeyMsg` rename implies Bubble Tea v2)
- §2d + §4 (Lip Gloss v2 dark/light handling)

Tasks:
- [ ] Update module deps in `go.mod` from v1 paths to v2 module paths:
  - `github.com/charmbracelet/bubbles` -> `charm.land/bubbles/v2`
  - `github.com/charmbracelet/bubbletea` -> `charm.land/bubbletea/v2`
  - `github.com/charmbracelet/lipgloss` -> `charm.land/lipgloss/v2`
- [ ] Run `go mod tidy` to refresh `go.sum`.
- [ ] Rewrite imports in:
  - `internal/app/config_wizard.go`
  - `internal/app/fix_tui.go`
  - `internal/app/fix_tui_wizard.go`
  - `internal/app/fix_tui_test.go`
  - `internal/app/config_wizard_test.go`
  - `internal/app/fix_tui_stage_stash_summary_test.go`
  - `internal/app/ui_badge.go`

Dependency note:
- Do this first. Other API migrations cannot compile until these imports/modules switch.

---

## Phase B: Key event migration (`tea.KeyMsg` -> `tea.KeyPressMsg`)

Guide refs:
- §2a Global Patterns

Current local examples:
- `internal/app/fix_tui.go:921`, `internal/app/fix_tui.go:1206`
- `internal/app/config_wizard.go:472`, plus type assertions at `:631`, `:651`, `:733`, `:778`, `:856`, `:900`, `:1050`, `:1219`
- `internal/app/fix_tui_wizard.go:936`, `:1223`, `:1788`
- tests: `internal/app/config_wizard_test.go` (55 uses), `internal/app/fix_tui_test.go` (71 uses), `internal/app/fix_tui_stage_stash_summary_test.go` (2 uses)

Tasks:
- [ ] Replace type switches/cases from `tea.KeyMsg` to `tea.KeyPressMsg`.
- [ ] Replace type assertions `msg.(tea.KeyMsg)` with `msg.(tea.KeyPressMsg)`.
- [ ] Update function signatures currently typed as `tea.KeyMsg`:
  - `internal/app/fix_tui_wizard.go:936`
  - `internal/app/fix_tui_wizard.go:1223`
  - `internal/app/fix_tui_wizard.go:1788`
- [ ] Update all test input event construction from `tea.KeyMsg{...}` to `tea.KeyPressMsg{...}`.
- [ ] Re-run key map behavior tests for `key.Matches(...)` paths after conversion.

Dependency note:
- Do after Phase A; before component-level refactors to minimize compile noise.

---

## Phase C: `help` component API migration

Guide refs:
- §2b Width/Height setters/getters
- §3 Help
- §4 Light and Dark Styles
- §5 removed symbols (`Model.Width` field removed)

Current local examples:
- `internal/app/config_wizard.go:469` (`m.help.Width = msg.Width`)
- `internal/app/fix_tui.go:405-410` (`helpModel.Width = innerWidth`)
- `internal/app/fix_tui.go:979` (`target.help.Width = m.width`)
- `internal/app/fix_tui.go:1124` (`m.help.Width = msg.Width`)
- `internal/app/fix_tui_test.go:469-470` (`got.help.Width`)

Tasks:
- [ ] Replace help width field access:
  - writes: `model.Width = x` -> `model.SetWidth(x)`
  - reads: `model.Width` -> `model.Width()`
- [ ] Ensure copied help models in `footerHelpView` still get width applied via setter.
- [ ] Apply explicit help styles (required by v2 model):
  - set `m.help.Styles = help.DefaultStyles(isDark)` (or explicit light/dark variant)
  - do this when dark/light state is known (see Phase F).
- [ ] Update tests asserting help width to use `Width()`.

Dependency note:
- Depends on Phase F for final dark/light wiring.

---

## Phase D: `list` component API migration

Guide refs:
- §3 List
- §5 removed symbols (`DefaultStyles()` signature change)

Current local examples:
- `internal/app/fix_tui.go:1109` (`styles := list.DefaultStyles()`)

Tasks:
- [ ] Replace `list.DefaultStyles()` with `list.DefaultStyles(isDark)`.
- [ ] Verify no usage of removed list style fields:
  - `Styles.FilterPrompt`
  - `Styles.FilterCursor`
  - (audit result: currently none in repo)
- [ ] Verify no `list.NewModel` alias usage (audit result: none).

Dependency note:
- Requires Phase F (`isDark` source) for final shape.

---

## Phase E: `viewport` component API migration

Guide refs:
- §2b Width/Height setter-getter pattern
- §3 Viewport (constructor + width/height/yoffset)
- §5 removed symbols (`New(w,h)`, exported fields)

Current local examples:
- `internal/app/fix_tui_wizard.go:1843` (`viewport.New(width, height)`)
- `internal/app/fix_tui_wizard.go:1842`, `:1846`, `:1847` (`BodyViewport.Width/Height` fields)
- `internal/app/fix_tui_wizard.go:925`, `:927`, `:931`, `:933`, `:1845` (`BodyViewport.YOffset` field)
- `internal/app/fix_tui_wizard.go:1164`, `:1167` (`BodyViewport.Height` in paging math)
- tests:
  - `internal/app/fix_tui_test.go:3377`, `:3383`, `:3456`, `:3462`, `:3628`, `:3631`, `:3637` (`BodyViewport.YOffset`)

Tasks:
- [ ] Replace constructor:
  - `viewport.New(width, height)` -> `viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))`
  - or `viewport.New(); SetWidth/SetHeight` immediately
- [ ] Replace viewport field writes:
  - `BodyViewport.Width = width` -> `BodyViewport.SetWidth(width)`
  - `BodyViewport.Height = height` -> `BodyViewport.SetHeight(height)`
- [ ] Replace field reads:
  - `BodyViewport.Width` -> `BodyViewport.Width()`
  - `BodyViewport.Height` -> `BodyViewport.Height()`
  - `BodyViewport.YOffset` -> `BodyViewport.YOffset()`
- [ ] Keep `SetYOffset(...)` calls; only migrate direct field reads.
- [ ] Update viewport-related tests to assert via `YOffset()`.

Audit check:
- `HighPerformanceRendering` not used (no migration needed there).

---

## Phase F: Light/dark strategy + Lip Gloss v2 (`AdaptiveColor` removal)

Guide refs:
- §2d (`AdaptiveColor` removed)
- §4 Light and Dark Styles (recommended `tea.RequestBackgroundColor` + `tea.BackgroundColorMsg`)

Current local examples:
- `internal/app/config_wizard.go:223-231`, `:279`, `:285`, `:290`, `:295`, `:343`, `:375`, `:2114`
- `internal/app/fix_tui.go:793`
- `internal/app/fix_tui_wizard.go:1782`, `:1815`
- `internal/app/ui_badge.go:28-29`, `:33-34`, `:38-39`, `:43-44`, `:49`
- style-guide doc currently recommends AdaptiveColor:
  - `docs/CLI_UX_AND_STYLE_GUIDE.md:60`

Tasks:
- [ ] Introduce per-model theme state (`isDark bool`) in interactive models:
  - config wizard model
  - fix TUI model / boot flow where styles constructed
- [ ] Request background in Bubble Tea flow:
  - add `tea.RequestBackgroundColor` to `Init()` command chain
  - handle `tea.BackgroundColorMsg` in `Update(...)`
- [ ] Replace `lipgloss.AdaptiveColor{...}` in code with explicit light/dark selection pattern (for v2):
  - choose color at style-build time using `isDark` (or shared helper)
- [ ] Rebuild/apply component styles from `isDark`:
  - `help.DefaultStyles(isDark)`
  - `list.DefaultStyles(isDark)`
  - any explicit textinput/table style setup that depends on palette
- [ ] Keep behavior deterministic in tests:
  - either inject a `tea.BackgroundColorMsg` before assertions
  - or set default `isDark` in constructors and test both modes explicitly

Local implementation reference for this pattern:
- `references/vendor/bubbletea/color.go` (`RequestBackgroundColor`, `BackgroundColorMsg`)
- `references/vendor/lipgloss/color.go` (`LightDark(isDark bool)`)

---

## Phase G: Textinput/table/spinner sanity pass (used components with smaller deltas)

Guide refs:
- §3 Textinput
- §3 Table
- §3 Spinner
- §5 removed symbols

Audit result:
- textinput:
  - no `DefaultKeyMap` usage
  - no removed style fields (`PromptStyle`, `TextStyle`, etc.) usage
  - no direct width field usage
- table:
  - already using `SetWidth`/`SetHeight`
- spinner:
  - using `spinner.New(...)`, `model.Tick`, no removed `NewModel` or package-level `Tick()`

Tasks:
- [ ] Keep these as explicit checkboxes during implementation:
  - confirm no hidden compile errors after module switch
  - confirm no newly-required style wiring after Phase F

---

## Phase H: Docs migration tasks

Tasks:
- [ ] Update `docs/CLI_UX_AND_STYLE_GUIDE.md`:
  - replace “prefer `lipgloss.AdaptiveColor`” with v2-compatible dark/light rule
  - add `tea.RequestBackgroundColor` + `tea.BackgroundColorMsg` recommendation for TUIs
- [ ] Update docs that describe component usage in this repo:
  - `docs/implemented/003-CONFIG_COMMAND_SPEC.md`
  - `docs/implemented/006-SWITCH_TO_LIST.md`
  - ensure wording points to Bubbles v2 module path/concepts where relevant
- [ ] Decide policy for `AGENTS.md:21` link wording:
  - keep GitHub project link for docs only, or
  - add explicit note that code imports use `charm.land/*/v2`

---

## 3. Verification checklist

## Mechanical scans (must be clean)

- [ ] `rg "github.com/charmbracelet/bubbles" internal go.mod`
- [ ] `rg "github.com/charmbracelet/bubbletea|github.com/charmbracelet/lipgloss" internal go.mod`
- [ ] `rg "\\btea\\.KeyMsg\\b|tea\\.KeyMsg\\{" internal/app`
- [ ] `rg "\\.help\\.Width\\b|help\\.Width\\b" internal/app`
- [ ] `rg "BodyViewport\\.Width\\b|BodyViewport\\.Height\\b|BodyViewport\\.YOffset\\b|viewport\\.New\\([^)]*,[^)]*\\)" internal/app`
- [ ] `rg "lipgloss\\.AdaptiveColor" internal docs`
- [ ] `rg "list\\.DefaultStyles\\(\\)" internal/app`

## Build/tests

- [ ] `go test ./internal/app -count=1`
- [ ] `go test ./... -count=1`

## Manual interactive smoke

- [ ] `bb config` flow:
  - window resize updates help/table dimensions correctly
  - help footer renders with expected styles in both light/dark terminals
- [ ] `bb fix` flow:
  - list renders expected style palette
  - wizard viewport scrolling still works (`up/down/pgup/pgdown`)
  - help footer width clamp still works
  - visual diff shortcut unchanged (`alt+v`)

---

## 4. Suggested implementation sequence (to reduce churn)

1. Phase A (deps/imports) + immediate `go test ./internal/app` compile check.
2. Phase B (key event types) + tests for event-heavy files.
3. Phase C + E (help + viewport field/getter-setter migrations).
4. Phase F (theme/dark-light refactor + AdaptiveColor cleanup).
5. Phase D + G (list/textinput/table/spinner final API alignment).
6. Phase H docs cleanup.
7. Full verification suite.

Critical dependency edges:
- Phase F provides `isDark` input needed by C/D.
- Phase A must precede every code phase.

