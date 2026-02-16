# CLI Style Guide

This guide defines the interaction and visual standards for `bb` terminal UX.

Applies to:

- Interactive TUIs built with Bubble Tea + Bubbles + Lip Gloss.
- Non-interactive command output (briefly; see "Non-interactive output").

## Principles

- Calm by default: structured chrome, no log spam, stable layout.
- Keyboard-first: predictable keys and obvious focus.
- Safe by default: mutating actions require explicit intent.
- Vertical space is precious: prefer 1-line chrome and compact field headers.
- Works at small sizes: test at ~`80x24` and degrade gracefully down to ~`60` columns.
- Relevance over completeness: hide non-applicable/no-op sections instead of rendering placeholder copy.

## Layout (Chrome)

Interactive screens share the same skeleton:

- Outer padding: `1` row, `2` columns.
- Header: product badge + title + dynamic subtitle on a single line, separated by ` · ` (middle dot).
  - Standard for dense list/table screens: embed this header in the main panel top border (for example `╭─  bb  fix · <subtitle> ─…╮`) instead of rendering a separate standalone header row.
- Main: exactly one primary panel per screen/step.
- Footer: sticky help panel at the bottom. No other key legends.
- Callouts: warnings/errors render above the help panel (so keys stay visible).

Chrome spacing rules:

- No blank lines between chrome rows (header, tabs, main panel, callouts, help). Borders already consume rows; extra padding is waste.
- No trailing empty rows or whitespace after the help panel. The help panel is the visual bottom of the screen.
- If extra vertical space remains, expand the primary scroll/list viewport first; do not insert blank rows between the main panel and footer.
- Header should not wrap to multiple lines; prefer truncating the subtitle with `...` over spending another row.
- If tabs/steps are not present, the main panel starts immediately under the header line.
- For dense list workflows, use the embedded bordered header as the default pattern, and fall back to a standalone header row only when bordered embedding would make the layout less readable.

Example shape (schematic):

```text
bb  <title>  ·  <dynamic subtitle>
[Step] [Step] [Step]    (optional)
------------------------------
| Main panel content          |
| ...                         |
------------------------------
<callout area (optional)>
<help panel (sticky bottom)>
```

Avoid unconditional trailing newlines in `View()` output when they cause top borders to scroll off-screen in shorter terminals.
Avoid trailing spaces on any rendered line (especially the last line) because they can cause awkward wraps in narrow terminals.
Prefer expressive terminal glyphs (box-drawing borders, bullets, dots, radio/toggle symbols) when they improve scanability and alignment.

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
- Inside a field block: prefer `Label - short description` on one line, then the control/value on its own line.
- Do not add blank lines between bordered chrome elements (panels, help) just to "separate" them; the border is already the separator.
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
- Prefer terminal-portable chords for secondary actions (for example `alt+<letter>`), and avoid punctuation-heavy control chords that vary across emulators.
- Render shortcut labels with platform-appropriate symbols where possible: on macOS prefer glyphs like `⌥`/`⌃`/`⌘`; on other platforms prefer textual labels like `alt`/`ctrl`/`meta`.

Text input rule:

- When any text input is focused, treat printable keys as text input only. Do not bind single-letter shortcuts that steal characters.

### Footer Help Legend

- Use exactly one keyboard legend: the sticky global footer help panel.
- Never render a second key legend inside panel bodies, wizard steps, or details blocks.
- The legend must be state-aware: show only shortcuts that are currently available/effective for the active screen and focus.
- Order shortcuts by importance for the current context (primary action first, then navigation, then secondary actions).
- Use compact key labels with symbols and lowercase text (`←/→`, `↑/↓`, `enter`, `esc`, `ctrl+a` or macOS `⌃A`).
- Keep separator formatting consistent as ` • ` between shortcut entries.
- In collapsed mode, footer help must render as exactly one line; if it overflows, truncate that same line and place `…` at the end (never wrap to a second line).
- `?` must always toggle expanded/full footer help on every interactive screen/mode (including wizards and summaries).

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
| Name - Used for display and remote creation.
| [ my-repo-name______________ ]
| (error text when invalid)
```

Guidelines:

- Left border indicates the row boundary; focused row border uses `Accent`.
- Label is bold; description is `Muted`.
- If the description is long, it may wrap or drop to its own line (but default to the single-line header when it fits).
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
- If a primary mutating action depends on reviewing scrollable context, gate it behind an explicit `Review` state until the viewport reaches the bottom.
- Secondary utility actions that are orthogonal to form completion (for example external viewers) should prefer explicit keyboard shortcuts over additional focus stops.

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
- Make summary/status chips responsive: if boxed chips cannot fit in one row, keep the same chip content/order/colors and degrade only by removing borders and wrapping at chip boundaries.
- Keep the selected row visible while navigating; viewport must follow the cursor.
- If list height can change based on below-list details, reserve height against worst-case visible details so single-step cursor movement does not trigger sudden page re-bucketing (no large backward/forward viewport jumps on a one-row move).
- Keep list pages top-anchored and avoid artificial spacer rows between the main panel and footer; absorb available vertical space by sizing the list viewport itself.
- Avoid nested bordered containers for the same region (table inside panel inside another framed box) because it frequently causes wrap artifacts.
- When rendering custom list rows, leave at least one guard column so the rendered width stays strictly below viewport width (prevents terminal auto-wrap).

When you need rich styling, prefer a details panel below/alongside the list:

- Show "Action help" for the currently selected action.
- Use distinct label/value styling (muted label + higher-contrast value).
- Keep detail values complete: do not truncate selected-item details (for example full repo paths) just to fit width; wrap as needed and reclaim space by shrinking adjacent scrollable regions (table/viewport) first.
- Avoid vertical border glyphs (`│`) that visually merge with table columns.
- When multiple short metadata fields are shown for the selected row, prefer a single compact line with ` · ` separators (for example `State · Auto-push · Branch · Reasons · Selected fixes`) and keep the action explanation on its own line.

### Scroll Indicators (More Above/Below)

When a content region is scrollable (for example a viewport in a wizard/details panel), tell the user when context is clipped.

- Use a short, muted indicator line at the bottom and/or top of the scrollable region.
- Show indicators only when there is actually more content in that direction.
- Keep the wording consistent and explicit about what to do.

Examples:

```text
↓ More below
```

```text
↑ More above
```

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

### Conditional Sections (No-op Omission Rule)

- Render a section only when it contains relevant, actionable information for the current item/action.
- If a section has zero relevant entries, omit the entire section (title + description + body).
- Do not render placeholder filler like "none", "not applicable", or "no changes detected" inside optional detail blocks.
- Apply this consistently in wizards, details panes, and previews (for example commands/effects previews, changed-file lists, warnings).

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
- When a wizard step can include or skip a mutation-critical prerequisite (for example staging/committing before a first push), expose an explicit toggle and default it to the complete/safest path.
- Wizard summaries for mutating flows must include concrete artifacts created by the step (for example created commits with short SHA + subject), not just high-level action status.

## Copy (Language)

- Use user-facing terms, not internal IDs (for example "Enable auto-push" not `enable-auto-push`).
- Use consistent verbs:
  - `Continue`: move to the next step/screen (navigation). Avoid external side-effects.
  - `Save`: persist user input (usually to disk/config). Prefer when the primary effect is writing configuration.
  - `Apply`: perform the selected changes/fixes (usually mutating state outside the screen, like git operations or file edits).
  - `Cancel`: exit without applying changes (safe default focus for risky confirmations).
  - `Skip`: intentionally do nothing for the current item and move on (explicit no-op).
- Descriptions: short sentences without jargon.
- Avoid multiple "do the thing" verbs on the same screen; if you truly need both, qualify them (for example `Save config` vs `Apply fixes`).
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
