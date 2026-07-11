# Menubar project actions

## Brief

Add contextual Fix and Sync controls to actionable local projects in the macOS menubar. Global and row operations must show the active project, a spinner, and one concise progress line.

## Final behavior

- Go remains authoritative for action eligibility, risk, execution, and progress messages.
- Safe deterministic fixes expose a compact Fix control; multiple safe fixes expose Fix… selection.
- Syncable out-of-date local projects expose a compact Sync control.
- Global sync and row actions stream project-specific progress into the popup.
- One mutation runs at a time; remote-machine rows stay read-only.
- Completion refreshes live status; failures remain visible and attributable.

## Boundaries

In: action capability contract, targeted sync, structured operation events, Swift operation coordination/UI, docs, automated and live validation.

Out: risky one-click fixes, interactive fix wizards inside the menubar, cancellation/queue management, raw Git log console, remote-machine mutation.

## Criteria and verification

- Go reports only valid safe local actions: contract/unit and end-to-end CLI tests.
- Targeted sync mutates only the selected repository: isolated end-to-end test.
- Machine progress is structured, ordered, attributable, and failure-aware: CLI tests.
- Swift renders controls and per-project operation state from contracts: Swift package tests.
- Real popup shows live data, row controls, spinner/status during an operation: rebuilt local app and Computer validation.
- Repository quality gates pass: project-native format, lint/static analysis, full Go and Swift tests, builds.

## Execution shape

Tracer-bullet child issues, red tests first, atomic child commits, independent final review, live macOS acceptance, push.

