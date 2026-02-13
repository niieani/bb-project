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

## Catalog UX Standards

- Empty state must not appear blank.
- Empty state should include:
  - What a catalog is.
  - Immediate next action (`Down` to open add form).
  - Concrete example paths.
- Catalog editor should use the same field blocks and input containers as other steps.

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

## Consistency Checklist

Before shipping any new TUI screen:

- Uses shared palette and border styles.
- Distinguishes focused vs non-focused controls.
- Uses meaningful labels (no internal config keys in primary UI copy).
- Has non-empty empty states.
- Has clear per-screen key legend text matching actual behavior.
