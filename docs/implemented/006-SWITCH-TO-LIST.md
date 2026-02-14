I recommend replacing `bubbles/table` in `bb fix`.

**Why `bubbles/table` is fighting us**

- `bubbles/table` hard-truncates every cell with `runewidth.Truncate(...)` during row render, with no per-cell style hook and no wrap control:
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/table/table.go:434`
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/table/table.go:435`
- It renders rows as one joined styled string in a viewport, so ANSI-decorated cell content is fragile and can corrupt width/truncation behavior.
- This aligns with the artifacts you reported (odd wrapping, replacement glyphs, cramped truncation).

**Better primitives from vendor references**

- `bubbles/list` has fixed row-height + spacing via delegate (`Height()/Spacing()`), which gives predictable one-row rendering:
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/list/defaultitem.go:106`
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/list/defaultitem.go:121`
- `bubbles/list` pagination keeps cursor visible and handles viewport/page recalculation cleanly:
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/list/list.go:780`
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbles/list/list.go:798`
- Bubble Tea’s own `table-resize` example uses **lipgloss/table** for rendering, not `bubbles/table`:
  - `/Volumes/Projects/Software/bb-project/references/vendor/bubbletea/examples/table-resize/main.go:9`
- `lipgloss/table` has ANSI/wrap/offset/height behaviors explicitly tested:
  - `/Volumes/Projects/Software/bb-project/references/vendor/lipgloss/table/table_test.go:597`
  - `/Volumes/Projects/Software/bb-project/references/vendor/lipgloss/table/table_test.go:1081`
  - `/Volumes/Projects/Software/bb-project/references/vendor/lipgloss/table/table.go:223`

## Proposal (recommended)

Replace `bb fix` view layer with **`bubbles/list` + custom delegate** (not `bubbles/table`).

1. Keep current fix business logic (`selectedAction`, apply/apply-all, ignore, refresh) unchanged.
2. Replace table widget with `list.Model` (filter/title/status/pagination/help hidden; we keep global sticky footer help).
3. Implement a custom delegate rendering each repo row as a fixed 1-line “grid”:
   - `repo | branch | state | reasons | selected action`
   - per-column truncation done manually before styling
   - fixed column width allocation from viewport width
4. Keep `Left/Right` strictly for action cycling (disable list page-left/page-right defaults).
5. Add a detail strip below list for full selected row values (full reasons, full action label), so row stays compact and readable.
6. Keep color coding, but apply styles safely in delegate output and detail strip (no mixed ANSI-internal truncation).
