# Cobra Migration Plan for `bb` (Clean Break + Completions + Docs)

## Summary

Migrate `/Volumes/Projects/Software/bb-project/internal/cli` from manual argument parsing/help rendering to a Cobra command tree, intentionally adopting Cobra-native UX instead of preserving legacy help/error text, while keeping documented exit-code semantics (`0/1/2`). Add a first-class `completion` command and generated CLI docs/manpages as part of the same migration.

## Locked Decisions

- Compatibility mode: **break for clean Cobra** (no text-parity with current manual help/errors).
- Scope: **include shell completions and generated docs/manpages**.
- Keep business logic in `/Volumes/Projects/Software/bb-project/internal/app` unchanged; only CLI layer and docs tooling change.
- Keep exit code contract from README/spec: `0` success, `1` unsyncable outcome, `2` usage/hard failure.

## Public CLI Spec After Migration

1. Root command:

- `bb` with persistent flag `-q, --quiet`.
- Cobra-native `help` command and `--help/-h` behavior.
- Top-level command list: `init`, `scan`, `sync`, `status`, `doctor`, `ensure`, `repo`, `catalog`, `config`, `completion`.

2. Command argument/flag contract:

- `bb init [project]` with `--catalog`, `--public`, `--push`, `--https`.
- `bb scan` with repeatable `--include-catalog`.
- `bb sync` with repeatable `--include-catalog`, `--push`, `--notify`, `--dry-run`.
- `bb status` with `--json`, repeatable `--include-catalog`.
- `bb doctor` with repeatable `--include-catalog`.
- `bb ensure` with repeatable `--include-catalog`.
- `bb repo policy <repo> --auto-push=<true|false>` with explicit boolean value required.
- `bb catalog add <name> <root>`, `bb catalog rm <name>`, `bb catalog default <name>`, `bb catalog list`.
- `bb config` with no args.
- `bb completion [bash|zsh|fish|powershell]` outputs completion script to stdout.

3. Error/exit behavior:

- Unknown command/flag/arg validation errors: exit `2`.
- App-level hard errors: exit `2`.
- Unsyncable-returning app flows (`scan/sync/doctor/ensure`) propagate exit `1`.
- No legacy custom help-topic map (`"repo policy"` etc.); Cobra command/subcommand help is the source of truth.

## Important API / Interface / Type Changes

- Keep exported entrypoint `Run(args []string, stdout io.Writer, stderr io.Writer) int` in `/Volumes/Projects/Software/bb-project/internal/cli`.
- Add Cobra-based command builder in `/Volumes/Projects/Software/bb-project/internal/cli`:
- `newRootCmd(runtime *runtimeState) *cobra.Command` (internal).
- `runtimeState` struct to hold writers, quiet mode, lazy app init, and resolved exit code.
- Add CLI/app seam for testability:
- Internal interface in `/Volumes/Projects/Software/bb-project/internal/cli` representing app methods used by commands.
- Internal dependency container for `userHomeDir` and app factory injection in tests.
- Remove legacy manual parser/help types/functions from `/Volumes/Projects/Software/bb-project/internal/cli/cli.go`:
- `helpItem`, `helpField`, `helpTopics`, `printUsage`, `printTopicHelp`, `parse*Args`, `stripGlobalFlags`, `run*Subcommand` manual dispatch.
- Add docs generator command:
- `/Volumes/Projects/Software/bb-project/cmd/bb-docs/main.go` using `github.com/spf13/cobra/doc`.
- Generated outputs:
- Markdown docs in `/Volumes/Projects/Software/bb-project/docs/cli`.
- Manpages in `/Volumes/Projects/Software/bb-project/docs/man/man1`.

## TDD Implementation Plan

1. Add characterization tests for migration targets first:

- Update `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go` to assert new Cobra behavior contract instead of legacy wording.
- Add tests for exit-code mapping `0/1/2` with fake app responses.
- Add command parsing tests that assert parsed options passed to fake app for each command.

2. Introduce Cobra skeleton with zero business changes:

- Add `cobra` dependency in `/Volumes/Projects/Software/bb-project/go.mod`.
- Build root command and subcommands with `RunE` handlers calling existing app methods.
- Keep `/Volumes/Projects/Software/bb-project/cmd/bb/main.go` unchanged (`os.Exit(cli.Run(...))`).
- Make tests pass for root/help/unknown command and base exit-code behavior.

3. Migrate command handlers command-by-command:

- Replace manual parsing with Cobra `Args` validators and flags.
- Use `StringArray` for repeatable `--include-catalog`.
- Enforce required repo policy flag value via explicit parse/validation path.
- Remove old parser helpers once tests are green.

4. Add completion command:

- Add `/Volumes/Projects/Software/bb-project/internal/cli` completion subcommand with shell switch.
- Add tests covering each shell and invalid shell handling.

5. Add docs/manpage generation:

- Implement `/Volumes/Projects/Software/bb-project/cmd/bb-docs/main.go`.
- Add `/Volumes/Projects/Software/bb-project/justfile` target `docs-cli` to regenerate docs.
- Commit generated files in `/Volumes/Projects/Software/bb-project/docs/cli` and `/Volumes/Projects/Software/bb-project/docs/man/man1`.

6. Update human docs:

- Refresh command reference in `/Volumes/Projects/Software/bb-project/README.md`.
- Add completion usage snippet in README.
- Remove legacy custom-help expectations from docs.

7. Final verification:

- Run `go test ./...`.
- Run `just build`.
- Run smoke checks:
- `./bb --help`
- `./bb help sync`
- `./bb repo policy --help`
- `./bb completion zsh`
- `go run ./cmd/bb-docs`

## Test Cases and Scenarios

1. Unit tests in `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go`:

- `bb help` and `bb <cmd> --help` exit `0`.
- Unknown command exits `2`.
- Unknown flag/invalid args exit `2`.
- `--quiet` toggles verbosity on app stub.
- `scan/sync/doctor/ensure` propagate app return code `1`.
- `repo policy` requires `<repo>` and explicit `--auto-push=<bool>`.
- `config` rejects unexpected args.

2. E2E regression tests:

- Keep existing `/Volumes/Projects/Software/bb-project/internal/e2e/logging_test.go` quiet behavior passing.
- Add one CLI smoke e2e for completion command output non-empty and exit `0`.

3. Docs generation tests:

- Add test for docs generator package to verify output files are produced in temp dirs (non-repo-mutating).
- Optional check-mode test to fail when committed docs are stale.

## Assumptions and Defaults

- Assume clean-break permission includes changed help/error text and command ordering.
- Assume exit-code contract (`0/1/2`) remains mandatory because it is documented behavior.
- Assume generated docs and manpages are committed to the repo, not built only at release time.
- Assume no backward-compatibility shim layer will be kept; legacy manual parser/help code is removed entirely.
