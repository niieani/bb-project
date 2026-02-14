# Plan: `bb fix` Risk Confirmation Wizard + Safety Gating

## Summary

Implement a new confirmation wizard in interactive `bb fix` for risky actions, rename “autofix/autofixable” terminology to “fix/fixable,” add uncommitted-secret safety gating, and add optional pre-commit `.gitignore` generation for missing root `.gitignore` files.  
Wizard pages will be shown sequentially for selected repos that have risky actions, then a final summary screen will show what was applied/skipped/failed.

## Decisions Locked

1. Risky actions requiring wizard confirmation in interactive flow: `push`, `set-upstream-push`, `stage-commit-push`, `create-project`.
2. Wizard coverage: only selected repos with risky actions.
3. Safety blocking: only when secret-like files are in uncommitted changes (tracked modified or untracked), not historical committed files.
4. Non-secret noisy directories (for example `node_modules`) do not hard-block risky fixes by themselves.
5. Fields are optional per action context.
6. Add skip option per repo in wizard.
7. Add end-of-run summary screen.

## Implementation Plan

### 1. Terminology and UI copy cleanup

1. Update all user-facing “autofix/autofixable” wording to “fix/fixable” in:
   `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`  
   `/Volumes/Projects/Software/bb-project/internal/app/fix_tui_test.go`  
   `/Volumes/Projects/Software/bb-project/docs/CLI-STYLE.md`
2. Rename constant `AutoFixCommitMessage` to `DefaultFixCommitMessage` in:
   `/Volumes/Projects/Software/bb-project/internal/app/fix.go`  
   `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`

### 2. Add repo risk/introspection model

1. Introduce fix-risk data structures in a new file:
   `/Volumes/Projects/Software/bb-project/internal/app/fix_risk.go`
2. Capture for a repo:

- uncommitted changed file list
- per-file `+/-` stats
- secret-like changed files
- noisy changed paths that should usually be ignored (for example `node_modules`, `.venv`, build output folders)
- missing root `.gitignore`
- suggested ignore patterns (detected patterns only)

3. Use git commands through existing runner (`RunGit`) for:

- status parsing
- numstat parsing

4. Secret blocking will only use uncommitted changed paths.
5. Noisy-path detection is advisory for interactive flow and eligibility gating for non-interactive `stage-commit-push` only when root `.gitignore` is missing.

### 3. Secret-like blocking and action availability

1. Extend eligibility logic so `stage-commit-push` is unavailable when uncommitted secret-like files are present.
2. Treat secret-like uncommitted files as a hard block for commit-producing fixes.
3. Do not treat noisy directories (for example `node_modules`) as secret-like files.
4. Keep other risky actions eligible unless they would commit working tree changes.
5. Non-interactive rule for `bb fix <project> stage-commit-push`:

- if root `.gitignore` is missing and noisy uncommitted paths are detected, the action is ineligible with a clear message directing users to interactive `bb fix` (wizard) or manual `.gitignore` creation.
- do not auto-generate `.gitignore` implicitly in non-interactive flow.

6. Ensure this applies in both:

- interactive list eligibility
- non-interactive `bb fix <project>` action listing/apply path

7. Surface explicit reason text when action is blocked.

### 4. New wizard flow in interactive fix

1. Add wizard state machine to:
   `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`
2. On apply:

- current-row apply: route to wizard if selected action is risky
- apply-all: build queue for selected repos with risky actions and process sequentially

3. Wizard content per repo:

- repo/path/action context
- target branch (and push target where relevant)
- changed files with `+/-` stats
- warning block for secret-like uncommitted files
- warning block for noisy uncommitted paths when root `.gitignore` is missing
- optional fields by action:
  `stage-commit-push`: commit message (blank => auto), toggle `Generate .gitignore before commit` (only when root `.gitignore` missing and patterns detected)  
  `create-project`: visibility selector (`default/private/public`, default from config)

4. Wizard actions:

- Apply
- Skip this repo
- Cancel remaining queue

5. Keep UI style aligned with config wizard primitives (`renderFieldBlock`, toggles, input styles).

### 5. Apply execution contract changes

1. Refactor execution arguments into an options struct in:
   `/Volumes/Projects/Software/bb-project/internal/app/fix.go`
2. Pass options from wizard to apply path:

- commit message override
- create-project visibility override
- pre-commit `.gitignore` generation toggle + pattern set

3. Implement `.gitignore` generation only when root file is missing, and include detected patterns only.
4. Non-interactive execution never creates `.gitignore` automatically.
5. Make `create-project` honor default visibility from config when override is unset (instead of hardcoded private).

### 6. Final summary screen

1. After risky queue processing, show summary view in `fix` TUI:

- applied entries
- skipped entries
- failures
- any generated `.gitignore` / auto-message usage notes

2. Close summary and return to repo list (no forced quit).

### 7. Documentation alignment

1. Update wording and behavior notes in:
   `/Volumes/Projects/Software/bb-project/README.md`  
   `/Volumes/Projects/Software/bb-project/docs/CLI-STYLE.md`
2. Document:

- risky-action wizard behavior
- secret-like uncommitted blocking rule
- `.gitignore` auto-generation toggle behavior
- end summary screen behavior

## Public API / Interface Changes

1. No command name/action ID changes.
2. Interactive UX changes:

- risky actions now require wizard confirmation
- new skip-per-repo path
- post-run summary screen

3. Non-interactive behavior change:

- `stage-commit-push` can be ineligible when uncommitted secret-like files are present.
- `stage-commit-push` can be ineligible when root `.gitignore` is missing and noisy uncommitted paths are detected (no implicit file generation).

4. Internal interface change:

- fix execution functions in `/Volumes/Projects/Software/bb-project/internal/app/fix.go` accept structured options instead of plain commit message.

## Test Plan (TDD-first)

1. Add/adjust unit tests in:
   `/Volumes/Projects/Software/bb-project/internal/app/fix_actions_test.go`  
   `/Volumes/Projects/Software/bb-project/internal/app/fix_tui_test.go`
2. New tests for:

- renamed fixable wording
- risky action detection
- wizard queue sequencing for apply-all (risky-only repos)
- skip/cancel behavior in wizard
- summary screen rendering
- blocking when uncommitted secret-like files exist
- `.gitignore` toggle visibility only when missing root file + detected patterns

3. Add e2e coverage in:
   `/Volumes/Projects/Software/bb-project/internal/e2e/fix_test.go`
4. E2E scenarios:

- `stage-commit-push` not offered when uncommitted `.env` exists
- same repo becomes eligible once `.env` is unchanged/removed from uncommitted set
- `bb fix <project> stage-commit-push` returns ineligible when blocked
- interactive wizard offers `.gitignore` generation toggle when root `.gitignore` is missing and noisy paths are present
- non-interactive `bb fix <project> stage-commit-push` is ineligible for missing `.gitignore` + noisy paths, with guidance to run interactive flow or add `.gitignore` manually

5. Run targeted suites continuously while implementing:

- `go test ./internal/app -run Fix`
- `go test ./internal/cli -run Fix`
- `go test ./internal/e2e -run Fix`

6. Final pass:

- `go test ./...`

## Assumptions / Defaults

1. Risky action set is fixed to: `push`, `set-upstream-push`, `stage-commit-push`, `create-project`.
2. Secret-like baseline includes exact `.env` in any directory and a small high-confidence set of key/cert filenames/extensions; matching is only against uncommitted changed files.
3. `.gitignore` generation is minimal and detected-pattern-based (not template-heavy).
4. Noisy directories (for example `node_modules`) are not secret indicators; they trigger `.gitignore` guidance/generation pathways instead.
5. Wizard is interactive-only; no new CLI flags are added in this slice.
