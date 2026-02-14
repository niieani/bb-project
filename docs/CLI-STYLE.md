# CLI Style Guide

This guide defines the interaction and visual standards for `bb` terminal UX.

Applies to:

- Interactive TUIs built with Bubble Tea + Bubbles + Lip Gloss.
- Non-interactive command output (briefly; see "Non-interactive output").

## Principles

- Calm by default: structured chrome, no log spam, stable layout.
- Keyboard-first: predictable keys and obvious focus.
- Safe by default: mutating actions require explicit intent.
- Works at small sizes: test at ~`80x24` and degrade gracefully down to ~`60` columns.

## Layout (Chrome)

Interactive screens share the same skeleton:

- Outer padding: `1` row, `2` columns.
- Header: product badge + title + one-line subtitle.
- Main: exactly one primary panel per screen/step.
- Footer: sticky help panel at the bottom. No other key legends.
- Callouts: warnings/errors render above the help panel (so keys stay visible).

Example shape (schematic):

```text
bb  <title>
<one-line subtitle>

[Step] [Step] [Step]    (optional)
------------------------------
| Main panel content          |
| ...                         |
------------------------------
<callout area (optional)>

<help panel (sticky bottom)>
```

Avoid unconditional trailing newlines in `View()` output when they cause top borders to scroll off-screen in shorter terminals.

## Visual System

### Tokens (Colors)

Use a shared palette (prefer `lipgloss.AdaptiveColor`) with these semantics:

- `Text`: body copy.
- `Muted`: descriptions, hints, secondary metadata.
- `Border`: frames for panels and controls.
- `Panel BG`: subtle surface tint for panels.
- `Accent`: focus, active tab, selection emphasis.
- `Accent BG`: focused field background, selected chip background.
- `Success`: enabled/healthy state.
- `Warning`: cautionary state.
- `Danger`: errors and destructive actions.

Rules:

- Never rely on color alone to convey meaning; pair it with text or icons.
- Keep border style consistent (rounded panels by default).
- Prefer bold over bright colors to emphasize within a panel.

### Spacing Rhythm

- Between field blocks: exactly 1 blank line.
- Title/description to first field: 1 blank line.
- Inside a field block: label, then description, then control/value on its own line.
- Pills/chips: at least 2 spaces horizontal padding; at least 2 spaces between adjacent pills.

## Interaction Model

### Focus

- Only one focus group is visually focused at a time (tabs, list/table, form fields, action row).
- When focus changes, remove focused styling from the previous group.
- Focus state must be visible without reading the help legend.

### Default Keys

Use this baseline across TUIs unless there's a strong reason to diverge:

- `Up/Down`: move between vertical items/fields.
- `Left/Right`: change the value of the currently focused horizontal control, or switch steps when tabs are focused.
- `Space`: toggle boolean / cycle simple options (never "submit").
- `Enter`: advance/accept/open (never toggles booleans).
- `Esc`: back/cancel (non-destructive).
- `Ctrl+C`: quit (with confirmation if it would discard work).
- `?`: toggle extended help.

Text input rule:

- When any text input is focused, treat printable keys as text input only. Do not bind single-letter shortcuts that steal characters.

## Components & Patterns

### Tabs / Steps (Wizards)

Use tabs when a flow has multiple non-trivial steps.

- Tabs are visually distinct from panel borders (bordered "chips", not plain text).
- Active tab is bold; focused tab uses `Accent` + `Accent BG` and appears "connected" to the panel (open-bottom border).
- `Left/Right` switches steps only when tabs are focused.
- Switching steps keeps focus on the tabs (does not jump into form fields).

### Field Block (Forms)

Structure every editable row the same way:

```text
| Name
| Used for display and remote creation.
| [ my-repo-name______________ ]
| (error text when invalid)
```

Guidelines:

- Left border indicates the row boundary; focused row border uses `Accent`.
- Label is bold; description is `Muted`.
- Validation errors are short, specific, and tell the user how to fix the input.

### Text Inputs

- Always render inputs inside a bordered container.
- Focused input uses `Accent` border (and optional `Accent BG`).
- Placeholders are plain, human-readable examples (not internal keys).

### Enums (Bounded Options)

- Never use free-form text for bounded choices.
- Render enums as single-line pills/chips.
- Interactions: `Left/Right` changes the focused enum; `Space` may also cycle.
- Always communicate defaults explicitly.

Example:

```text
Visibility
Controls who can see the project.
  [● private (default)]  [○ public]
```

### Toggles (Booleans)

- Use switch-like pills: `● ON` and `○ OFF`.
- `ON` uses `Success` tone; `OFF` uses muted/neutral tone.
- The label/description always sits next to the toggle so meaning is visible.

### Buttons / Action Rows

Rules:

- Every action is keyboard focusable with a clear focus treatment.
- Use one focus treatment across primary/secondary/danger buttons.
- Keep labels width-stable between focused and unfocused states.

Example focus treatment:

```text
Actions: [Cancel]  Skip   Apply
```

Confirmation ordering:

- For potentially destructive confirmations, order left-to-right: `Cancel`, secondary escape hatch (for example `Skip`), primary action (for example `Apply`).
- Default focus is always `Cancel` to prevent accidental double-`Enter`.

### Lists & Tables

Rules:

- Default selections should be no-op (explicitly show `-` / "no action") rather than auto-selecting a mutating action.
- Keep table cell content plain text (avoid embedding ANSI/lipgloss inside cell strings); style at the row/column layer instead.
- Keep the selected row visible while navigating; viewport must follow the cursor.
- Avoid nested bordered containers for the same region (table inside panel inside another framed box) because it frequently causes wrap artifacts.
- When rendering custom list rows, leave at least one guard column so the rendered width stays strictly below viewport width (prevents terminal auto-wrap).

When you need rich styling, prefer a details panel below/alongside the list:

- Show "Action help" for the currently selected action.
- Use distinct label/value styling (muted label + higher-contrast value).
- Avoid vertical border glyphs (`│`) that visually merge with table columns.

### Badges / Chips (Non-interactive)

Use badges for compact metadata, not controls.

- Single-line, bold, padded (`" LABEL "`).
- Short labels (usually <= 14 chars) and consistent semantics:
  - Neutral/info: informational metadata.
  - Success: completed/safe.
  - Warning: caution.
  - Danger: blocking/critical.
- In aligned lists, reserve a fixed-width badge slot so rows stay column-aligned when a badge is absent.

### Callouts (Errors / Warnings)

- Render callouts above the footer help panel.
- Keep them short: what happened, why it matters, what to do next.
- Never dump stack traces or raw git output into the panel; summarize and offer next steps.

### Empty States

Empty states should never be blank. Include:

- A one-sentence explanation of what the screen manages.
- The immediate next action (keys included).
- A concrete example.

### Loading / Startup

- Never show a blank screen with only a cursor while work is in progress.
- Loading view includes:
  - Spinner.
  - One stable context sentence.
  - One live status line driven by real execution events (replace the line as steps change).
- Keep stderr/log noise out of the TUI surface; map progress into the loading view.

### Long Lists (Changed Files, Results, etc.)

- Render as one item per row.
- When showing file changes, include `+/-` counts when available (and use `Success`/`Danger` tones).
- Cap long lists and say so explicitly: "showing first N of M".
- Never let a long list push essential controls (like confirmations) off-screen.

### Multi-step Wizards

- Use tabs or a clear step header when steps are non-trivial.
- Place progress on the title line and align it to the top-right when possible (for example `1/3`).
- Budget height from full chrome (header + borders + footer) so the top panel border is never clipped off-screen.

## Copy (Language)

- Use user-facing terms, not internal IDs (for example "Enable auto-push" not `enable-auto-push`).
- Use consistent verbs:
  - Buttons: `Apply`, `Continue`, `Save`, `Cancel`, `Skip`.
  - Descriptions: short sentences without jargon.
- Always explain what will change before running a mutating action.
- Prefer "fix/fixable" over legacy "autofix/autofixable".

## Non-interactive Output

- Print human output to stdout; errors to stderr; exit non-zero on failure.
- Keep output scannable: short headings, one item per line, minimal noise.
- Respect non-color environments (`NO_COLOR`) and avoid encoding meaning only via color.

## Checklist (Before Shipping)

- Layout: header + single main panel + sticky footer help; no nested frames.
- Focus: exactly one focused group; focus is obvious without reading help.
- Keys: baseline keys match behavior; help legend matches reality.
- Safety: destructive actions require confirmation; default focus is safe.
- Width/height: no hard-wrap artifacts at ~`80x24`; content truncates/caps with explicit messaging.
- Copy: no internal keys/IDs in user-facing strings; errors include next steps.

## Local References

If you need examples for borders/tabs/layout:

- `references/vendor/lipgloss/examples/layout/main.go`
- `references/vendor/lipgloss/borders.go`
- `references/vendor/bubbles/table/table.go`
