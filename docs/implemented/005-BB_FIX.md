# Plan: `bb fix` Interactive Inspector + Actionable Repo Fixes

## Summary

Implement `bb fix` as the single entrypoint for interactive repo inspection and targeted remediation.

`bb fix` (no project) opens an interactive Bubble Tea table of repos with context-aware fix actions.

`bb fix <project>` shows the repo’s current state and currently eligible fixes.

`bb fix <project> <action>` applies one fix, then re-observes and reports resulting state.

This ships the balanced first action set you selected: `push`, `stage-commit-push`, `pull-ff-only`, `set-upstream-push`, `enable-auto-push`, `abort-operation`, plus interactive-only `ignore` (session-only).

## Public APIs / Interfaces / Types

1. CLI command surface in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`.
   `bb fix [--include-catalog <name> ...]`
   `bb fix <project> [--include-catalog <name> ...]`
   `bb fix <project> <action> [--message <text>|--message=auto] [--include-catalog <name> ...]`
2. New action names are stable kebab-case identifiers.
   `push`, `stage-commit-push`, `pull-ff-only`, `set-upstream-push`, `enable-auto-push`, `abort-operation`, `ignore` (interactive only)
3. New app-layer types in `/Volumes/Projects/Software/bb-project/internal/app/fix.go`.
   `type FixOptions struct {...}`
   `type FixAction struct {...}`
   `type FixTarget struct {...}`
4. New git helpers in `/Volumes/Projects/Software/bb-project/internal/gitx/git.go`.
   `AddAll`, `Commit`, `MergeAbort`, `RebaseAbort`, `CherryPickAbort`, `BisectReset`

## Fix Action Eligibility and Execution Rules

| Action              | Eligible when                                                                              | Execution                                                                                                     |
| ------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| `ignore`            | Interactive mode only; always available for selected row                                   | Session-only mute in TUI state; no file writes                                                                |
| `abort-operation`   | `operation_in_progress != none`                                                            | `git merge --abort` or `git rebase --abort` or `git cherry-pick --abort` or `git bisect reset`                |
| `push`              | No operation; origin+upstream exist; `ahead > 0`; not diverged                             | `git push`                                                                                                    |
| `stage-commit-push` | No operation; origin exists; dirty tracked or untracked; not diverged                      | `git add -A`; `git commit -m <msg>`; push (`git push` or `git push -u origin <branch>` when upstream missing) |
| `pull-ff-only`      | No operation; clean tree; upstream exists; `behind > 0`; `ahead == 0`; not diverged        | optional `git fetch --prune` when enabled; `git pull --ff-only`                                               |
| `set-upstream-push` | No operation; origin exists; branch exists; upstream missing                               | `git push -u origin <branch>`                                                                                 |
| `enable-auto-push`  | Repo has `repo_key`; repo metadata exists; `auto_push=false`; usually `push_policy_blocked` | Update repo metadata `auto_push=true`                                                                         |

Global gating rule from your requirement: when operation is in progress, only `abort-operation` (and interactive `ignore`) are shown.

## Command Behavior

1. `bb fix` requires interactive terminal and opens TUI.
2. `bb fix <project>` resolves project by exact path, then exact `repo_key`, then unique repo `name`; ambiguity is an error listing candidates.
3. `bb fix <project> ignore` is rejected with clear message: interactive-only action.
4. `--message` handling for `stage-commit-push`.
   `--message=<text>` uses custom message.
   `--message=auto` or omitted uses auto message template: `bb: checkpoint local changes before sync`.
5. After applying any action, the target is re-observed immediately.
6. Exit codes.
   `0`: action/list succeeded and repo is syncable after action (or list command).
   `1`: command succeeded but repo remains unsyncable or action currently ineligible due state.
   `2`: usage/parse/unknown action/execution failure.

## Interactive UX Spec (`bb fix`)

1. Table-first layout in `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go` using Bubble Tea + Bubbles table/help/key.
2. Default sort is unsyncable first, then name, then path.
3. Default scope is all repos in selected catalogs; unsyncable-first ordering satisfies triage.
4. Fix selection is interactive in the table row itself: each row has a selected fix "cell" value that the user can change with arrow keys.
5. `←/→` cycles the selected fix for the currently highlighted row, only across currently eligible actions for that repo.
6. Row-level preference is remembered for the session (`map[repoKey]selectedActionIndex`), so moving up/down between repos preserves each row's selected fix.
7. Key bindings.
   `↑/↓` select repo row, `←/→` cycle selected fix for current row, `enter` apply current row's selected fix, `r` refresh state, `i` ignore selected repo for session, `u` unignore/show ignored, `?` help, `q` quit.
8. On refresh or after apply, action eligibility is recomputed; if the previously selected fix is no longer valid for a row, selection falls back to the first eligible action.
9. `stage-commit-push` opens one-line message prompt prefilled with auto message; unchanged value is treated as auto mode.
10. Ignore is session-only in-memory state and never persisted.

## Implementation Plan by File

1. `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`
   Add `fix` help topic, usage lines, parser for `FixOptions`, and dispatch to app `RunFix`.
2. `/Volumes/Projects/Software/bb-project/internal/app/app.go`
   Add `RunFix(opts FixOptions)` wiring and any app struct fields needed for fix TUI runner.
3. `/Volumes/Projects/Software/bb-project/internal/app/fix.go`
   Implement target discovery, selector resolution, eligibility computation, action execution, and post-action re-observation/reporting.
4. `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`
   Implement interactive table/action model with session ignore and action execution loop.
5. `/Volumes/Projects/Software/bb-project/internal/gitx/git.go`
   Add missing git primitives required by fix actions.
6. `/Volumes/Projects/Software/bb-project/README.md`
   Document `bb fix` command forms, action names, and interactive-only `ignore`.
7. `/Volumes/Projects/Software/bb-project/docs/SPEC.md`
   Update non-goal wording to clarify: auto-commit is still not part of autonomous sync, but explicit `bb fix stage-commit-push` is an opt-in manual command.

## TDD Test Cases and Scenarios

1. CLI parser tests in `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`.
   Covers command forms, `--message` validation, interactive-only `ignore`, selector/action arg errors.
2. Eligibility unit tests in `/Volumes/Projects/Software/bb-project/internal/app/fix_actions_test.go`.
   Covers each action’s positive/negative gating and “operation in progress limits action list” rule.
3. E2E tests in `/Volumes/Projects/Software/bb-project/internal/e2e/fix_test.go`.
   `bb fix` non-interactive fails with TTY message.
   `bb fix <project>` lists correct eligible actions.
   Interactive TUI supports row-local `←/→` action cycling and preserves per-row selection when moving between rows.
   After state refresh/action apply, invalid row selection is clamped/fallback to first eligible action.
   `push` clears ahead when possible.
   `stage-commit-push` works with auto and custom message.
   `set-upstream-push` sets upstream and pushes.
   `pull-ff-only` fast-forwards behind-only repo.
   `enable-auto-push` toggles repo policy.
   `abort-operation` clears rebase/merge/cherry-pick/bisect markers.
   Selector resolution supports path/repo_key/name and errors on ambiguity.
4. Full regression suite remains green via `go test ./...` after each feature slice.

## Suggested Additional Fixes (not in first shipping slice)

`set-origin` (interactive prompt for missing origin), `discard-tracked`, `clean-untracked`, and `stash-and-sync` are useful but intentionally excluded now because they are higher-risk/destructive.

## Assumptions and Defaults

`bb fix` is the hub command (no separate `inspect` command).
`ignore` is interactive-only and session-only.
Default interactive list is all repos with unsyncable-first ordering.
Balanced action set ships in v1 of this feature.
`stage-commit-push` supports both auto and custom commit message modes.
No persistent ignore metadata is introduced.
