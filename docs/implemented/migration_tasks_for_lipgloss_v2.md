# Lip Gloss v2 Migration Tasks

Source of truth: `references/vendor/lipgloss/UPGRADE_GUIDE_V2.md`.

Goal: migrate from `github.com/charmbracelet/lipgloss` v1 to `charm.land/lipgloss/v2` using the full migration path (explicit color/background handling), not temporary shortcuts.

## 0) Audit Snapshot (current repo)

### Code files with direct Lip Gloss usage
- `internal/app/config_wizard.go` (import + 16x `lipgloss.AdaptiveColor`)
- `internal/app/fix_tui.go` (import + 1x `lipgloss.AdaptiveColor` + 3x `lipgloss.TerminalColor`)
- `internal/app/fix_tui_wizard.go` (import + 2x `lipgloss.AdaptiveColor`)
- `internal/app/ui_badge.go` (import + 9x `lipgloss.AdaptiveColor`)
- `internal/app/fix_tui_test.go` (import + removed APIs: `ColorProfile` / `SetColorProfile`)

### Module/dependency files
- `go.mod` (`github.com/charmbracelet/lipgloss v1.1.0`)
- `go.sum` (v1 checksums)

### Docs with v1-era guidance/wording
- `docs/CLI_UX_AND_STYLE_GUIDE.md` (`lipgloss.AdaptiveColor` recommendation)
- `docs/implemented/006-SWITCH_TO_LIST.md` (mentions `lipgloss/table`; should use v2 path wording)

### Removed APIs inventory (good news)
No code usage found for:
- `DefaultRenderer`, `SetDefaultRenderer`, `NewRenderer`, `renderer.NewStyle`
- `WithWhitespaceForeground`, `WithWhitespaceBackground`
- `HasDarkBackground()` no-arg form, `SetHasDarkBackground`
- root `CompleteColor` / `CompleteAdaptiveColor`

Guide refs: §11 Removed APIs.

---

## 1) Phase Order / Dependencies

1. Dependency + import path migration.
2. Color system migration (`AdaptiveColor`/`TerminalColor` removal).
3. Background detection integration in Bubble Tea models.
4. Test migration for removed profile APIs.
5. Docs updates.
6. Verification gates (targeted + full).

Dependency notes:
- Do not convert `AdaptiveColor` call sites before introducing a shared color-selection strategy (otherwise repeated ad-hoc logic).
- Do not finalize tests until color/background strategy is stable.

---

## 2) Module Path + Dependency Tasks

- [ ] Replace module dependency in `go.mod`:
  - From: `github.com/charmbracelet/lipgloss v1.1.0`
  - To: `charm.land/lipgloss/v2 v2.0.0`
- [ ] Update imports:
  - `internal/app/config_wizard.go:16`
  - `internal/app/fix_tui.go:19`
  - `internal/app/fix_tui_wizard.go:17`
  - `internal/app/fix_tui_test.go:15`
  - `internal/app/ui_badge.go:6`
- [ ] Run `go mod tidy` and ensure `go.sum` no longer contains v1 module lines.
- [ ] Grep gate: zero matches for `github.com/charmbracelet/lipgloss`.

Guide refs: §2 Module Path, §12 Quick Reference.

---

## 3) Color System Migration (full path)

Preferred approach per guide: explicit `LightDark`/`Complete` usage instead of `compat` quick path.

### 3.1 Introduce shared color selection helpers

- [ ] Add a central UI color helper module (suggested: `internal/app/ui_theme.go`) that:
  - accepts explicit `bgIsDark bool`
  - returns semantic colors as `color.Color`
  - uses `lipgloss.LightDark(bgIsDark)(light, dark)` for dual-theme tokens
- [ ] Move existing semantic palette (`textColor`, `mutedTextColor`, `borderColor`, `accentColor`, etc.) behind that helper.

Local examples to refactor:
- `internal/app/config_wizard.go:223-231`
- `internal/app/fix_tui.go:670-806` (style vars consuming shared tokens)
- `internal/app/ui_badge.go:24-50`

Guide refs: §3 Color System (AdaptiveColor replacement), §6 Background Detection.

### 3.2 Replace all `lipgloss.AdaptiveColor` usage

- [ ] Convert every `lipgloss.AdaptiveColor{...}` occurrence to explicit color selection from the shared helper.
- [ ] Files to touch:
  - `internal/app/config_wizard.go` (16 occurrences)
  - `internal/app/fix_tui.go` (1 occurrence)
  - `internal/app/fix_tui_wizard.go` (2 occurrences)
  - `internal/app/ui_badge.go` (9 occurrences)

Migration delta example:
- v1: `Foreground(lipgloss.AdaptiveColor{Light: "#0969DA", Dark: "#58A6FF"})`
- v2 full path: `Foreground(theme.Accent())` where `theme.Accent()` uses `LightDark`.

Guide refs: §3 Color System, §6 Background Detection.

### 3.3 Remove `TerminalColor` API usage

- [ ] Replace `lipgloss.TerminalColor` with `image/color.Color`.
- [ ] Add `import "image/color"` where needed.
- [ ] Refactor:
  - `internal/app/fix_tui.go:1751` (`fixSummarySegment.Fg`)
  - `internal/app/fix_tui.go:3004` (`renderFixSummaryChip`)
  - `internal/app/fix_tui.go:3018` (`renderFixSummaryPill`)

Guide refs: §3 Color System (`TerminalColor` removed), §11 Removed APIs.

---

## 4) Bubble Tea Background Detection Tasks

Lip Gloss v1 implicitly resolved adaptive colors; v2 requires explicit background context.

- [ ] Add background-color request in TUI init commands:
  - `internal/app/config_wizard.go:460` (`Init`)
  - `internal/app/fix_tui.go:1115` (`fixTUIModel.Init`)
  - evaluate `fixTUIBootModel.Init` at `internal/app/fix_tui.go:896` (pass-through strategy)
- [ ] Handle `tea.BackgroundColorMsg` in update loops and rebuild styles when background changes:
  - `internal/app/config_wizard.go:464`
  - `internal/app/fix_tui.go:1119`
- [ ] Ensure style reinitialization is centralized (no duplicated style constructors across update paths).

Guide refs: §6 Background Detection and Adaptive Colors.

---

## 5) Printing / Downsampling Verification Tasks

Even if Bubble Tea manages terminal rendering, verify output profile behavior after migration.

- [ ] Confirm whether current Bubble Tea version (`v1.3.10`) provides adequate downsampling with Lip Gloss v2 in this app runtime.
- [ ] If not, set explicit colorprofile-backed output writer for program output.
- [ ] Add/adjust at least one integration assertion to catch profile-related regressions (truecolor vs downgraded output expectations).

Local code contexts:
- `internal/app/config_wizard.go:380` (`tea.NewProgram`)
- `internal/app/fix_tui.go:812` (`tea.NewProgram`)

Guide refs: §5 Printing and Color Downsampling.

---

## 6) Test Migration Tasks

### 6.1 Remove deleted profile APIs from tests

- [ ] Rewrite `TestRenderPanelWithTopTitleUsesPanelTopBorderColor` in `internal/app/fix_tui_test.go:843+`:
  - remove `lipgloss.ColorProfile()` and `lipgloss.SetColorProfile(...)`
  - remove `github.com/muesli/termenv` test dependency if unused after rewrite
- [ ] Keep semantic assertion: top border corner uses accent/top-border color.

Guide refs: §4 Renderer Removal, §11 Removed APIs.

### 6.2 Regression coverage for color strategy

- [ ] Add/adjust tests to validate style token selection after background detection update.
- [ ] Cover both dark and light paths for at least one wizard/list render path.

Suggested focus files:
- `internal/app/fix_tui_test.go`
- `internal/app/config_wizard_test.go`

Guide refs: §3, §6.

---

## 7) Documentation Tasks

- [ ] Update `docs/CLI_UX_AND_STYLE_GUIDE.md:60`:
  - from `lipgloss.AdaptiveColor` recommendation
  - to explicit v2 guidance (`LightDark`-based semantic tokens, explicit background context)
- [ ] Update `docs/implemented/006-SWITCH_TO_LIST.md:19-24` wording for v2 module path:
  - mention `charm.land/lipgloss/v2/table` for import examples
- [ ] Add migration note in README/changelog if project policy expects dependency upgrades to be documented.

Guide refs: §2 Module Path, §3 Color System, §6 Background Detection.

---

## 8) Verification / Exit Criteria

### 8.1 Automated checks

- [ ] `go test ./internal/app -run 'FixTUI|ConfigWizard' -count=1`
- [ ] `go test ./...`
- [ ] `go test ./... -race` (if part of standard gate)
- [ ] `rg -n "github.com/charmbracelet/lipgloss|lipgloss\.AdaptiveColor|lipgloss\.TerminalColor|ColorProfile\(|SetColorProfile\("` returns zero matches outside `references/`.

### 8.2 Manual smoke checks

- [ ] Run interactive config wizard; validate colors/readability on light + dark terminal themes.
- [ ] Run interactive fix TUI; validate borders, chips, selected row, wizard badges, summary pills.
- [ ] Confirm no layout regressions at small terminal sizes (`~80x24`, `~60 cols`).

### 8.3 Done definition

- [ ] Compiles/tests green.
- [ ] No v1 import/API symbols remaining.
- [ ] Docs reflect v2 color guidance.
- [ ] No compatibility shims (`compat`) left in final code unless explicitly justified.

---

## 9) Local Code Examples Index (for implementer)

- Import path v1 usage:
  - `internal/app/config_wizard.go:16`
  - `internal/app/fix_tui.go:19`
  - `internal/app/fix_tui_wizard.go:17`
  - `internal/app/fix_tui_test.go:15`
  - `internal/app/ui_badge.go:6`
- `AdaptiveColor` hotspots:
  - `internal/app/config_wizard.go:223-231,279,285,290,295,343,375,2114`
  - `internal/app/fix_tui.go:793`
  - `internal/app/fix_tui_wizard.go:1782,1815`
  - `internal/app/ui_badge.go:28-29,33-34,38-39,43-44,49`
- `TerminalColor` hotspots:
  - `internal/app/fix_tui.go:1751,3004,3018`
- Removed renderer/profile API usage in tests:
  - `internal/app/fix_tui_test.go:844-847`

