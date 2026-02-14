# CLI Style Guide (Bubble Tea + Bubbles + Lip Gloss)

This document defines the visual language for interactive CLI surfaces in `bb`, including `bb config` and `bb fix`.

## Goals

- Make the wizard feel structured and calm, not like raw terminal output.
- Keep information hierarchy obvious: app chrome, step chrome, fields, and help.
- Preserve keyboard-first interaction while making focus state unmistakable.
- Reuse one design system across all Bubble Tea command UIs.

## Lip Gloss Inspiration Sources

Patterns in this guide are based on local references under `references/vendor/lipgloss`:

- `examples/layout/main.go`
  - Custom tab borders (`tabBorder`, `activeTabBorder`), tab gaps, card/panel framing, consistent spacing.
- `examples/layout/main.go`
  - Status nuggets/pills for compact state indicators.
- `borders.go`
  - Rounded and normal borders as primary framing primitives.
- `table/table.go`
  - Header/cell/selected row style layering for tabular content.

## Design Tokens

Use centralized tokens (AdaptiveColor where possible):

- Text: high-contrast foreground for body copy.
- Muted text: secondary instructions and long descriptions.
- Border: neutral frame color for all cards and controls.
- Panel background: subtle surface tint.
- Accent: focus/tab/selection emphasis.
- Accent background: focused field and selected enum chip background.
- Success: enabled checkbox state.
- Danger: validation and error callouts.

## Layout Rules

- Outer document padding: `1` row, `2` columns.
- Header block:
  - Left badge for product (`bb`).
  - Title + one-line subtitle.
  - Use the same header chrome across interactive commands (for example `config`, `fix`).
- Step navigation:
  - Lip Gloss border tabs, active tab visually distinct.
  - Focused active tab uses highlighted background and open-bottom border.
- Main content:
  - One rounded panel per step.
  - Step title + short explainer first.
- Footer:
  - Dedicated help panel.
  - Error/confirm-warn callouts above help.
  - The footer help panel is the single source of truth for keybindings.
  - Footer help must be visually anchored to the terminal bottom (no trailing empty rows beneath it).
  - Root `View()` output should not append an unconditional trailing newline; this can push top borders out of viewport in some terminals.

## Spacing System

Use explicit rhythm values and keep them consistent across all wizard steps:

- Field block gap: exactly one blank line between adjacent field blocks.
- Section preface gap: title/description to first field uses one blank line.
- Control spacing inside a field:
  - Label -> description: next line.
  - Description -> control value: next line.
- Chip/pill controls:
  - Minimum horizontal padding: 2 spaces on both sides.
  - Minimum gap between adjacent enum chips: 2 spaces.
  - Do not mix multi-line pills with single-line helper text in one baseline row; use all pills or move helper text to its own line.

## Field Components

### Field Block

Use a reusable field block for all form rows:

- Left accent border to indicate row boundaries.
- Focused row: border color changes to accent.
- Internal structure:
  - Bold label.
  - Muted description.
  - Control/value (input, enum, checkbox, list).
  - Optional error text.

### Action Buttons

- For list-management steps (for example Catalogs), provide explicit action buttons below the list.
- Primary action uses emphasized style (for example `Continue`).
- Secondary actions use neutral style (for example `Add`, `Set Default`).
- When items exist, include `Edit` as the first action and focus it by default.
- Destructive action uses danger style and requires explicit confirmation before applying.
- Buttons must be keyboard-focusable and show focus state clearly.
- Use one shared focus treatment for all button variants (secondary, primary, danger):
  - Focused button uses a more vivid fill than its non-focused variant.
  - Focused button keeps underline enabled as an additional cue.
  - Focused button uses symmetric label markers (`[Label]`) to avoid one-sided visual imbalance.
- Do not use asymmetric one-sided focus markers for button labels.
- Keep focused and non-focused labels width-stable by reserving two wrapping characters for both states (`[Label]` when focused, ` Label ` when not).

### Input

- Render text inputs inside a bordered container.
- Focused input border uses accent color.
- Keep placeholders plain and human-readable.

### Enums

- Never use free-form text for bounded enums.
- Render as single-line selectable pills (`● option` active, `○ option` inactive) with:
  - Active chip: accent-tinted background + bold foreground.
  - Inactive chip: neutral surface + muted text.
- Interaction:
  - `Left/Right` cycles current enum value.
  - `Space` also cycles for consistency with toggles.

### Checkboxes / Toggles

- Use switch-like pills: `● ON` and `○ OFF`.
- `ON` uses success foreground with subtle success background.
- `OFF` uses muted foreground with neutral background.
- Row label and description always adjacent to token.

### Badges / Chips

- Use badges for short, non-interactive metadata tags (for example `AUTO-IGNORE`, risk flags, or compact status labels).
- Badge shape:
  - Single-line only.
  - Bold text.
  - Horizontal padding of one space on each side.
- Keep badge labels short and scannable (prefer uppercase tokens, usually <= 14 chars).
- Use semantic badge tones consistently:
  - Neutral: informational metadata with no positive/negative implication.
  - Info: contextual hints.
  - Success: completed/safe/healthy state.
  - Warning: cautionary state (for example noisy files to be ignored).
  - Danger: blocking/error-critical state.
- When rendering badges inline in aligned lists/tables, reserve a fixed-width badge slot so rows remain column-aligned when a badge is absent.
- Badges are not focus targets; interactive controls should use the button/toggle focus patterns instead.

## Catalog UX Standards

- Empty state must not appear blank.
- Empty state should include:
  - What a catalog is.
  - Immediate next action (`Down` to open add form).
  - Concrete example paths.
- Catalog editor should use the same field blocks and input containers as other steps.

## Fix UX Standards

- In fix-selection tables, default each row to explicit no-op (`-`) rather than auto-selecting a mutating action.
- Provide a batch operation to apply all explicitly selected fixes.
- Keep table cell content plain text (no inline ANSI/lipgloss-rendered spans), otherwise width math can break and rows may soft-wrap incorrectly.
- Use responsive table columns; `Reasons` and `Selected Fix` should expand in wider terminals instead of truncating aggressively.
- If category color-coding is needed, apply it in surrounding detail panels or status chips, not directly in table cell strings.
- Keep the selected row visible while navigating large lists; viewport must follow cursor movement.
- Avoid nested bordered containers for the same content region (for example list-inside-list-panel) because combined frame widths can trigger wrap artifacts (`─┘`, double-height rows).
- For custom row delegates, reserve at least one wrap-guard column so rendered row width stays strictly below viewport width.
- Keep tiering internal for sort/grouping; in the table itself communicate fixability via state wording and color, not a dedicated `Tier` column.
- `fixable` may be shown only when available fix actions cover all current unsyncable reasons; otherwise show manual/blocked.
- Order rows by fixability tier: fixable unsyncable first, unsyncable manual/blocked second, syncable last.
- Action labels shown in UI must be human-readable (for example `Allow auto-push in sync`) rather than raw internal action IDs (for example `enable-auto-push`).
- When multiple fix actions are available for a repo, include an explicit `All fixes` option in `Selected Fix`.
- In selected-row details, always render a concise `Action help` line that explains what the currently selected fix action will do.
- In selected-row details, avoid field-border glyphs (`│`) to prevent visual merging with table columns.
- In selected-row details, render labels and values with distinct styles (for example accent label + higher-contrast value + muted path/context).
- List height must be budgeted from full chrome (header, panel borders, details, footer help) so top panel borders are never clipped off-screen.
- In confirmation wizards, place progress badges (for example `1/3`) on the same top row as the title and align them to the top-right edge.
- For risky fix confirmation actions, order buttons left-to-right as `Cancel`, `Skip`, `Apply`, and default focus to `Cancel` to prevent accidental double-`Enter` applies.
- Changed-files sections in fix wizards should render as explicit lists (one file per row), with colored `+`/`-` counters and a cap + indicator when rows are trimmed.
- Prefer trimming with explicit `showing first N of M` messaging over overflowing content; never let long file lists collapse surrounding form controls.
- Create-project visibility pickers should be two-option horizontal controls (`private`, `public`) with explicit default labeling (for example `private (default)`), and left/right should change value when that control is focused.
- In create-project confirmations, include a dedicated editable repository-name field with an empty value and placeholder fallback to current folder/repo name.

## Startup/Loading Standards

- Never show a blank interactive terminal with only a blinking cursor while work is in progress.
- For any interactive screen that performs startup work before the main UI is usable, render a loading view immediately on entry.
- Loading view must include:
  - A spinner (or equivalent animated progress indicator).
  - A stable one-line context sentence describing what is happening overall.
  - A live status line that updates in real time with the current startup step.
- The live status line should come from real execution events (for example internal progress/log steps), not synthetic timers.
- The latest observed step should always replace the previous line so users can see forward progress.
- Keep verbose/stderr log noise out of the TUI surface; in interactive mode, map those progress events into the loading UI instead of printing raw logs.

## Interaction + Focus Standards

- Only one element group may be visually focused at a time (for example tabs, table row set, button row, or editor controls).
- When focus changes, remove active/focused styling from the previously focused group.
- `Up/Down` moves between fields.
- `Esc` goes back/cancel.
- `Enter` advances/applies (never toggles boolean state).
- `Space` toggles booleans.
- `Left/Right` changes steps only when tabs are focused.
- When switching tabs while tabs are focused, keep tabs focused.
- Do not render per-step/per-panel key legends inside content; use only the global sticky footer legend.
- In list views, `Enter` on a selected row opens contextual edit UI.
- In button groups and action rows, `Left/Right` moves focus between adjacent actions.
- When any text input is focused, alphabetic keys must be treated as text input only (never as command shortcuts like quit/navigation).
- In multi-control wizards, use explicit focus movement (`up/down` or `tab`) between input fields, enum pickers, and action buttons; left/right should only operate on the currently focused horizontal control.

## Consistency Checklist

Before shipping any new TUI screen:

- Uses shared palette and border styles.
- Distinguishes focused vs non-focused controls.
- Uses meaningful labels (no internal config keys in primary UI copy).
- Has non-empty empty states.
- Has clear per-screen key legend text matching actual behavior.
