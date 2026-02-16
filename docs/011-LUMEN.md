# Lumen Integration Spec for `bb` (Decision-Complete)

## Summary

This spec adds optional Lumen integration to `bb` with symbolic UI affordances, strict CLI AI-generation behavior, and direct passthrough-style commands.

The key outcomes are:

1. Symbolic commit-message generation button in `bb fix` wizard (`✨`) that runs immediately, shows spinner while running, and writes generated text into the commit message input for user review/edit.
2. Symbolic visual diff button (`◫`) in `bb fix` wizard that launches Lumen diff UI in the repo context.
3. New CLI AI flag for fix: `--ai-message`, mutually exclusive with `--message`.
4. New passthrough commands: `bb diff <project> ...args` and `bb operate <project> ...args`.
5. Global config for optional integration and install tip visibility, with disabled-tip support.
6. All Lumen usage guarded by availability/config; missing/error paths show install/config tip messaging.

## Public API / Interface Changes

### CLI surface

1. Add `bb fix --ai-message` in `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`.
2. Enforce mutual exclusion: `--ai-message` cannot be combined with `--message`.
3. Add new command `bb diff <project> ...args`.
4. Add new command `bb operate <project> ...args`.
5. `bb diff` and `bb operate` are raw passthrough style without `--` separator (as requested).

### Config schema

1. Extend config model in `/Volumes/Projects/Software/bb-project/internal/domain/types.go` with:

```yaml
integrations:
  lumen:
    enabled: true
    show_install_tip: true
```

2. Add defaults in `/Volumes/Projects/Software/bb-project/internal/state/store.go`.
3. Add load/save compatibility in `/Volumes/Projects/Software/bb-project/internal/state/store.go` for missing legacy fields.

### TUI affordances

1. Add `✨` commit generation button in fix wizard commit section.
2. Add `◫` visual diff launch button in fix wizard changed-files context.
3. Add spinner state for commit generation; button turns into spinner while executing.

## Detailed Behavior Spec

## 1) Lumen capability resolution and tip policy

1. Add centralized helper module at `/Volumes/Projects/Software/bb-project/internal/app/lumen.go`.
2. Capability is resolved from:

- Config: `integrations.lumen.enabled`.
- Binary presence: `exec.LookPath("lumen")`.

3. If integration is disabled, all Lumen paths are treated as unavailable.
4. If unavailable or failing, tip is shown only when `integrations.lumen.show_install_tip=true`.
5. Tip copy (single canonical function) is:
   `tip: install lumen with 'brew install jnsahaj/lumen/lumen'; for AI features run 'lumen configure'.`
6. Tip disable copy references config key:
   `set integrations.lumen.show_install_tip=false to hide this tip.`

## 2) `bb fix` wizard: immediate AI commit generation (`✨`)

1. Scope: only actions with commit message field enabled (`stage-commit-push`, `publish-new-branch`, `checkpoint-then-sync`).
2. UI behavior:

- Render commit input and symbolic button `✨` on same control block.
- Button is focusable and pressable via Enter.
- On press, button enters loading state with spinner glyph replacing `✨`.
- While generating, wizard controls are locked except passive rendering.
- On success, commit input value is replaced with generated message.
- Focus returns to commit input so user can tweak before Apply.
- On failure, commit input value remains unchanged and tip/error is shown.

3. Generation algorithm (decision-complete):

- Snapshot current index state (`.git/index` bytes or missing-state marker).
- Run `git add -A` in repo.
- Run `lumen draft` in repo, capture stdout/stderr.
- Restore original index snapshot in `defer` path, regardless of success/failure.
- Normalize output by trimming whitespace and taking full line output as commit message.
- Reject empty output as error.

4. Rationale:

- Lumen draft depends on staged diff.
- Immediate generation is required.
- Index snapshot/restore guarantees no persistent staged-state mutation before Apply.

## 3) `bb fix` CLI: `--ai-message` behavior

1. `--ai-message` is available for non-interactive fix flows.
2. `--ai-message` is valid only for commit-producing fix actions.
3. If used with unsupported action, return validation error (exit 2).
4. `--ai-message` and `--message` together is invalid (exit 2).
5. Execution behavior:

- At commit step, generate via same Lumen helper logic.
- Inject generated message into commit operation.

6. Failure mode (locked decision):

- Fail fast if Lumen unavailable or generation fails.
- No fallback to static default message.
- Return actionable error with tip (if tip enabled).

## 4) New command: `bb diff <project> ...args`

1. Purpose: launch Lumen visual diff viewer in target repo.
2. Resolution:

- Resolve `<project>` using existing project/repo selector resolution used by `bb info`/`bb fix`.
- Must resolve to local repo path.

3. Execution:

- Run subprocess: `lumen diff <args...>` with `cwd=<resolved repo path>`.
- Inherit stdio for interactive UI.

4. Arg forwarding:

- Raw passthrough, no separator required.
- All tokens after `<project>` are forwarded verbatim to `lumen diff`.

5. Parsing strategy:

- Command disables Cobra flag parsing for this command so `--watch`, `--focus`, etc. are not consumed by `bb`.
- Manual help handling for `-h/--help`.

6. Failure paths:

- If Lumen unavailable/disabled, command exits non-zero with tip.
- If subprocess exits non-zero, bubble up error and include tip when relevant.

## 5) New command: `bb operate <project> ...args`

1. Purpose: launch Lumen operate workflow in target repo.
2. Resolution:

- Same selector resolution as `bb diff`.

3. Execution:

- Run subprocess: `lumen operate <args...>` with `cwd=<resolved repo path>`.
- Inherit stdio so user interacts directly with Lumen confirm prompt.

4. Arg forwarding:

- Raw passthrough without `--`.
- All tokens after `<project>` are forwarded verbatim.

5. Parsing strategy:

- Disable Cobra flag parsing on this command.
- Manual help handling for `-h/--help`.

6. Failure paths:

- Same guard/tip behavior as `bb diff`.

## 6) Config wizard + docs

1. Add integrations controls to `bb config` wizard in `/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`.
2. Wizard includes two toggles:

- Enable Lumen integration.
- Show Lumen install/config tips.

3. Update config review diff output to include integration changes.
4. Update docs in:

- `/Volumes/Projects/Software/bb-project/README.md`
- `/Volumes/Projects/Software/bb-project/docs/cli/bb_fix.md`
- `/Volumes/Projects/Software/bb-project/docs/cli/bb.md`
- New generated pages for `bb diff` and `bb operate`.

## Implementation Map (Files)

1. `/Volumes/Projects/Software/bb-project/internal/domain/types.go`

- Add integration config structs/fields.

2. `/Volumes/Projects/Software/bb-project/internal/state/store.go`

- Default values, load compatibility, save behavior.

3. `/Volumes/Projects/Software/bb-project/internal/app/lumen.go` (new)

- Capability detection, tip builder, subprocess helpers, draft generation helper with index snapshot/restore.

4. `/Volumes/Projects/Software/bb-project/internal/app/fix.go`

- Add `AIMessage` option field.
- Integrate AI generation into commit step when requested.
- Enforce fail-fast semantics.

5. `/Volumes/Projects/Software/bb-project/internal/app/fix_tui_wizard.go`

- Add `✨` button UI/focus/state/spinner.
- Add immediate generation async message flow.
- Add `◫` diff button UI/focus and launch handling.

6. `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`

- Update wizard help map/status interactions for new focus areas.

7. `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`

- Add `--ai-message` flag and mutual exclusion validation.
- Add `diff` and `operate` commands with raw passthrough handling.

8. `/Volumes/Projects/Software/bb-project/internal/app/app.go`

- Add `RunDiff` and `RunOperate` app methods.

9. `/Volumes/Projects/Software/bb-project/internal/app/config.go`

- Validate and persist new integration fields if needed.

10. `/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`

- Add integrations step controls and review diff text.

11. `/Volumes/Projects/Software/bb-project/README.md` and generated CLI docs

- Document new flags/commands/config and examples.

## Test Cases and Scenarios

1. Config default/load tests in `/Volumes/Projects/Software/bb-project/internal/state/store_test.go`.

- Missing `integrations` yields defaults `enabled=true`, `show_install_tip=true`.
- Explicit false values persist.

2. CLI parsing tests in `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`.

- `--ai-message` parsed correctly.
- `--ai-message` + `--message` rejected.
- `bb diff` forwards raw args after project.
- `bb operate` forwards raw args after project.

3. Fix apply tests in `/Volumes/Projects/Software/bb-project/internal/app/fix_apply_test.go` and related.

- AI generation success commits generated text.
- AI generation unavailable/failure returns error and does not commit.
- Unsupported action + `--ai-message` fails validation.

4. TUI tests in `/Volumes/Projects/Software/bb-project/internal/app/fix_tui_test.go`.

- `✨` button appears only when commit field enabled and Lumen integration available.
- Pressing `✨` enters spinner state then updates commit input.
- On generation error, existing message retained and tip shown.
- `◫` button launches diff command path and returns to wizard state.

5. Command execution tests for new app methods.

- `RunDiff` resolves project and invokes `lumen diff` in repo cwd.
- `RunOperate` resolves project and invokes `lumen operate` in repo cwd.
- Unavailable Lumen returns tip-aware error.

6. Docs smoke test.

- Regenerated docs include `bb diff`, `bb operate`, and updated `bb fix` flag docs.

## Acceptance Criteria

1. In interactive `bb fix`, user can click `✨`, see spinner, receive generated commit message in input, edit it, then apply.
2. In interactive `bb fix`, user can click symbolic diff button `◫` to launch Lumen diff and return to wizard.
3. If Lumen is missing/erroring, pressing `✨` or `◫` shows actionable install/config tip (unless disabled by config).
4. CLI supports `bb fix ... --ai-message` with strict fail-fast behavior.
5. CLI supports `bb diff <project> ...args` and `bb operate <project> ...args` raw passthrough style without `--`.
6. Tip visibility is globally controllable via config.
7. All updated docs and tests pass.

## Assumptions and Defaults

1. Symbols are fixed as `✨` for AI generation and `◫` for visual diff.
2. `integrations.lumen.enabled` defaults to `true`.
3. `integrations.lumen.show_install_tip` defaults to `true`.
4. `--ai-message` is strict and never silently falls back.
5. `bb diff` and `bb operate` use raw passthrough without `--`, with command-level flag parsing disabled.
6. Immediate TUI generation uses index snapshot/restore to avoid persistent pre-apply staging side effects.
