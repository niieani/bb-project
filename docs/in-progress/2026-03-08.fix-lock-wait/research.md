# Research

## User report

- Repro: scheduled background `bb sync` holds global lock; user runs interactive `bb fix`.
- Current outcome:
  - stderr/stdout prints `another bb process holds the lock`
  - shell also shows raw terminal escape replies like `^[[?2026;2$y`, `^[[?2027;1$y`, `^[]11;rgb:1010/1010/1010^G`

## Relevant code

- Lock storage/acquire:
  - `internal/state/store.go`
- Interactive fix bootstrap + TUI:
  - `internal/app/fix.go`
  - `internal/app/fix_tui.go`
- Existing tests:
  - `internal/state/store_test.go`
  - `internal/app/fix_tui_test.go`
  - `internal/app/fix_interactive_test.go`
  - `internal/e2e/sync_edge_test.go`

## Findings

### 1. Lock metadata incomplete

Current lock payload only stores:

- `pid`
- `hostname`
- `created_at`

No command/reason field. Caller cannot say whether lock owner is `sync`, `scan`, `fix`, etc.

### 2. Interactive `bb fix` does real loading during Bubble Tea boot

`runFixInteractive` starts a Bubble Tea boot model. Boot `Init()` immediately:

- starts spinner
- starts repo loading
- requests terminal background color

Repo loading calls into `loadFixReposForInteractive`, which tries to acquire the global lock. If another process holds the lock, boot exits early with error.

### 3. Raw escape garbage likely terminal capability replies

Bubble Tea v2 startup requests terminal capabilities:

- `OSC 11` background-color query from `tea.RequestBackgroundColor`
- mode 2026 / 2027 queries from Bubble Tea startup (`RequestModeSynchronizedOutput`, `RequestModeUnicodeCore`)

Refs:

- local module: `/Users/bbrzoska/go/pkg/mod/charm.land/bubbletea/v2@v2.0.0/color.go`
- local module: `/Users/bbrzoska/go/pkg/mod/charm.land/bubbletea/v2@v2.0.0/tea.go`

Interpretation:

- `11;rgb:1010/1010/1010` => terminal background-color response
- `?2026;2$y` / `?2027;1$y` => terminal mode reports

Likely failure mode: boot exits on lock contention before all terminal replies are consumed/drained; shell then prints leftover replies.

### 4. Non-interactive and interactive lock behavior should diverge

- Non-interactive commands should still fail fast on active lock.
- Interactive `bb fix` should not fail fast; better UX:
  - enter TUI
  - show loading/wait state
  - explain which command owns the lock
  - show elapsed duration
  - retry until lock released

## Constraints / design implications

- Need backward-compatible parsing of old lock files without command metadata.
- Need stale-lock recovery to keep working.
- Need deterministic tests for elapsed time/status text; inject clock where needed.
- Need loading screen copy to follow `docs/CLI_UX_AND_STYLE_GUIDE.md`: single main panel, compact chrome, clear status line.
