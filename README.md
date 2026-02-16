# bb

`bb` is a local-first CLI that helps keep Git repositories consistent across multiple machines.

It automates:

- repository bootstrap (`git init`, remote setup, metadata registration)
- discovery of repos under configured catalog roots
- safe cross-machine convergence (branch/fast-forward when syncable)
- unsyncable state reporting and notifications

State replication is intentionally externalized (Syncthing, Dropbox, iCloud, rsync, etc.). `bb` reads and writes YAML state files; your sync tool moves them between machines.

## Status

This repository implements the v1 specification in `docs/1.0/SPEC.md`.

Known hardening work planned for v1.1 is documented in `docs/PLAN-V1.1.md`.

## Core Model

- `repo_key`: catalog/path identity (`<catalog>/<relative-path>`) used for repo metadata and convergence
- `origin identity check`: normalized `origin_url` comparison used for target-path safety checks
- `catalog`: named local root where repos live
- `machine file`: one YAML per machine (catalogs + observed repo states)
- `repo metadata`: shared per-repo YAML (name, visibility, policy)
- `syncable`: safe for automation
- `unsyncable`: requires manual intervention first

For each `repo_key`, `bb` picks the newest syncable observation as winner and tries to converge local copies when safe.

## Requirements

- macOS/Linux shell environment
- Go `1.26.0` (for building/testing)
- `just` in `PATH`
- `git` CLI in `PATH`
- `gh` CLI in `PATH` for real `bb init` GitHub repo creation
- External file sync tool for `~/.config/bb-project`

## Build

```bash
just build
```

Run:

```bash
./bb <command> [args]
```

## Developer Install

Build a dev binary and place a symlink in a directory that is already on your `PATH`:

```bash
just install-dev ~/bin
```

This creates `~/bin/bb -> /absolute/path/to/repo/.dist/dev/bb`.

Default link directory is `.bin` inside the repo:

```bash
just install-dev
```

Remove a symlink with:

```bash
just uninstall-dev ~/bin
```

## First-Time Setup

Run interactive setup:

```bash
./bb config
```

This wizard configures:

- `github.owner` (required)
- default visibility and remote protocol
- sync/notify options
- Lumen integration defaults (install tips and optional AI commit generation when commit message is empty)
- catalogs, per-catalog repository layout depth (`1` or `2`), and default catalog

Lumen note:

- The `Fixes` tab includes `Use Lumen for empty commit messages`.
- If `lumen` is not available on `PATH`, this toggle is disabled and the wizard shows an install/config tip.

Manual catalog commands remain available.

## Command Reference

Global flags:

- `--quiet` / `-q`: suppress verbose `bb:` logs

Top-level commands:

- `version`
- `init`
- `clone`
- `link`
- `info`
- `diff`
- `operate`
- `scan`
- `sync`
- `status`
- `doctor`
- `ensure`
- `scheduler`
- `fix`
- `repo`
- `catalog`
- `config`
- `completion`

### `bb version`

Print build metadata:

- semantic version (`dev` for local builds)
- commit SHA (`unknown` for local builds)
- build timestamp (`unknown` for local builds)

### `bb init [project] [flags]`

Initialize or adopt a repo in a catalog and register metadata.

Flags:

- `--catalog <name>` or `--catalog=<name>`
- `--public` (default visibility is private)
- `--push` (force initial push/upstream setup)
- `--https` (use HTTPS remote protocol instead of SSH)

Behavior:

- Selected catalog is `--catalog` when provided, otherwise the machine default catalog.
- If `project` is provided, target path is `<selected-catalog-root>/<project>`.
- If `project` is omitted, infers project root from current directory only when current directory is inside the selected catalog subtree and matches its layout depth.
- `project` must match the selected catalog layout depth:
  - depth `1`: `repo`
  - depth `2`: `owner/repo`
- Initializes git repo if missing.
- Creates GitHub repo via `gh repo create` (unless running in test backend mode).
- Streams `gh`/`git` command output directly to the terminal during remote creation.
- Sets/verifies `origin`.
- Creates/updates repo metadata YAML.
- Does not run an automatic post-init `bb scan`.

### `bb clone <repo> [flags]`

Clone an existing repository into a configured catalog and immediately register metadata/state.

Accepted `<repo>` forms:

- `org/repo`
- GitHub HTTP/HTTPS repository link (`https://github.com/org/repo`)
- HTTPS/SSH git URL

Flags:

- `--catalog <name>` (overrides `clone.default_catalog`)
- `--as <catalog-relative-path>`
- `--shallow` / `--no-shallow`
- `--filter <spec>` / `--no-filter`
- `--only <path>` (repeatable sparse checkout paths)

Behavior:

- Uses clone defaults from `clone.*` config, then applies catalog preset mapping from `clone.catalog_preset`, then applies explicit CLI flags.
- Fails when target path conflicts and no `--as` is provided.
- If repository already exists locally (same origin identity), command is a no-op and prints existing location.

### `bb link <project-or-repo> [flags]`

Create a symlink to a project/repository under a target directory (defaults to `references`).

Flags:

- `--as <link-name>`
- `--dir <target-dir>`
- `--absolute`
- `--catalog <name>` (used for auto-clone fallback)

Behavior:

- When run inside a git repo, anchors links at repo root; otherwise anchors at current directory.
- Resolves local selectors first (`repo_key`, `catalog:project`, unique name).
- If selector is a repo input and no local match exists, auto-clones first using clone defaults.
- Existing same-target symlink is treated as no-op; conflicting existing paths fail.

### `bb info <project-or-repo>`

Show resolved local details for a project/repository selector.

Behavior:

- Uses the same selector resolution rules as `bb link` (local selectors first, then repo-origin resolution).
- If project exists in state but local git clone is missing, returns exit code `1`.
- If selector cannot be resolved to a local project, returns exit code `1`.

### `bb diff <project> ...args`

Launches Lumen visual diff in the selected local repository.

Behavior:

- Uses local selector resolution (`path`, `repo_key`, unique repo name).
- Forwards all args after `<project>` directly to `lumen diff`.
- Fails fast if Lumen is unavailable or disabled.

### `bb operate <project> ...args`

Launches Lumen operate workflow in the selected local repository.

Behavior:

- Uses local selector resolution (`path`, `repo_key`, unique repo name).
- Forwards all args after `<project>` directly to `lumen operate`.
- Fails fast if Lumen is unavailable or disabled.

### `bb scan [--include-catalog <name> ...]`

Discovers git repos under selected catalogs, observes git state, and writes machine observations.

Exit code is `1` when at least one observed repo is unsyncable.

### `bb sync [flags]`

Performs full convergence flow:

1. observe local repos
2. publish machine observations
3. load all machine and repo metadata state
4. reconcile by winner
5. pull/checkout and clone only when safe and allowed
6. optionally emit notifications

Flags:

- `--include-catalog <name>` (repeatable)
- `--push` (allow pushing ahead commits when repo policy blocks by default)
- `--notify` (emit deduped unsyncable notifications)
- `--notify-backend <stdout|osascript>` (override notification backend; falls back to `BB_NOTIFY_BACKEND`, then `stdout`)
- `--dry-run` (observe/reconcile decisions without write-side sync actions)

Additional behavior:

- Reconcile target catalog is strict: `repo_key` catalog must match a local catalog mapping.
- Missing local mapping for a remote-known catalog is skipped with warning (no cross-catalog fallback).
- `--include-catalog` for a catalog known on other machines but missing locally returns a hint to map catalogs via `bb config`.
- Clone during sync is controlled per catalog by `auto_clone_on_sync` (default off).

Exit code is `1` only when selected catalogs still contain **blocking** unsyncable repos after sync.
Non-blocking reasons (`clone_required`, `catalog_not_mapped`) do not force exit code `1`.

### `bb status [--json] [--include-catalog <name> ...]`

Shows last recorded machine repo state.

- plain mode: one line per repo
- `--json`: machine + repo list JSON output

### `bb doctor [--include-catalog <name> ...]`

Prints unsyncable repos and reasons from machine file.

- refreshes local observations only when the last scan snapshot is stale (default threshold: 60 seconds; configurable via `sync.scan_freshness_seconds`)

Returns `1` if any unsyncable repo is present in selected catalogs.

### `bb ensure [--include-catalog <name> ...]`

Alias for sync convergence (`bb sync` with include filters).

### `bb scheduler`

Manage macOS launchd scheduling for periodic sync.

- `bb scheduler install [--notify-backend <stdout|osascript>]`
  - installs/replaces a LaunchAgent that runs `bb sync --notify --quiet`
  - reads `scheduler.interval_minutes` from config
  - defaults scheduled backend to `osascript` unless overridden by flag or `BB_NOTIFY_BACKEND`
- `bb scheduler status`
  - reports whether LaunchAgent is installed and current interval/backend
- `bb scheduler remove`
  - unloads and removes the LaunchAgent

### `bb fix [project] [action] [flags]`

Inspect repositories and apply context-aware fixes.

Forms:

- `bb fix` opens interactive table mode (requires interactive terminal).
- `bb fix <project>` prints repo state and currently eligible fixes.
- `bb fix <project> <action>` applies one action and re-observes state.

Interactive apply behavior:

- Risky fixes (`push`, `sync-with-upstream`, `set-upstream-push`, `stage-commit-push`, `create-project`) open a confirmation wizard before execution.
- Wizard shows changed files with `+/-` stats, target branch context, and a per-repo skip option.
- For commit-producing actions, wizard includes commit message input with symbolic `✨` generation (Lumen draft).
- When changed files are shown, press `⌥V` on macOS (or `alt+v` on other platforms) to launch Lumen visual diff and return to the same wizard state.
- Wizard can generate a minimal root `.gitignore` when missing.
- In list mode, when repository details wrap (for example long paths or action-help text), `bb fix` shrinks the table viewport first so top chrome and footer help remain visible without truncating details text, and keeps one-row navigation stable (no sudden page jump when moving by one row).
- In list mode, the primary panel uses a compact titled border (`bb fix · Interactive remediation for unsyncable repositories`) to preserve vertical space, and selected-repo metadata is rendered on one dot-separated line.
- The list-mode summary stats row is responsive: it renders pill boxes when they fit, and automatically switches to a wrapped dot-separated key/value line on narrow terminals to prevent horizontal overflow artifacts.
- Selected-repo metadata wrapping is segment-aware (wraps at ` · ` boundaries), so labels stay attached to their values and avoid orphan trailing tokens on separate lines.
- In list mode, `bb fix` keeps the primary panel top-anchored and places footer help immediately below it (no artificial spacer gap between panel and footer); available height is absorbed by list sizing.
- In list mode, `enter` runs currently selected fixes; when none are selected, it runs the currently browsed fix for the selected repo.
- Interactive list ordering is by catalog (default catalog first), then `fixable`, `unsyncable`, `not cloned`, `syncable`, and `ignored`; repos with `clone_required` are surfaced as `not cloned`.
- Before computing fix eligibility, `bb fix` re-probes repositories whose cached `push_access` is `unknown`.
- For GitHub origins (including `*.github.com` aliases), the probe treats `gh` viewer permission as authoritative when available; it falls back to `git push --dry-run` only when `gh` cannot determine access.
- Repositories that still have `push_access=unknown` after probing do not get push-related fix actions; run `bb repo access-refresh <repo>` after resolving probe blockers.
- The startup loading screen shows phase-based progress and collapses noisy multiline probe/auth errors into concise status text while checks continue.

Selector resolution for `<project>`:

- exact local path
- exact `repo_key`
- unique repo name

Flags:

- `--include-catalog <name>` (repeatable)
- `--message <text>` (used with commit-producing fix actions; pass `auto` to use the configured empty-message default behavior)
- `--ai-message` (generate commit message with Lumen for commit-producing actions)
- `--sync-strategy <rebase|merge>` (used with `sync-with-upstream`; default `rebase`)

`--message` and `--ai-message` are mutually exclusive.

Actions:

- `clone`
- `push`
- `sync-with-upstream`
- `create-project`
- `stage-commit-push`
- `pull-ff-only`
- `set-upstream-push`
- `enable-auto-push`
- `abort-operation`
- `ignore` (interactive mode only, session-only)

Safety gating:

- `stage-commit-push` is blocked when secret-like uncommitted files are detected (for example `.env`).
- In non-interactive flow, `stage-commit-push` is also blocked when root `.gitignore` is missing and noisy uncommitted paths are detected (for example `node_modules`).
- `stage-commit-push` is blocked when branch is behind upstream (run `sync-with-upstream` first).
- Push-producing fixes are blocked when cached push access is `unknown` or `read_only`.

### `bb repo policy <repo> --auto-push=<false|true|include-default-branch>`

Updates `auto_push` mode in repo metadata:

- `false`: disable auto-push
- `true`: allow auto-push on non-default branches
- `include-default-branch`: allow auto-push on any branch, including default branch

`<repo>` selector can be either:

- exact `repo_key`
- repo `name` (must not be ambiguous)

### `bb repo remote <repo> --preferred-remote=<name>`

Sets the repo-level preferred remote used when `bb` needs to choose a remote for operations (for example upstream setup and branch tracking).

### `bb catalog` subcommands

- `bb catalog add <name> <root>`
- `bb catalog rm <name>`
- `bb catalog default <name>`
- `bb catalog list`

### `bb config`

Launches an interactive Bubble Tea wizard for onboarding and reconfiguration.

- edits all `config.yaml` keys
- edits this machine's catalogs (including layout depth, clone preset mapping, default branch auto-push defaults, and per-catalog sync clone policy via `auto_clone_on_sync`) and default catalog
- can be rerun to change existing values
- requires an interactive terminal

Onboarding support:

- catalogs known from other machines are shown as remote-only rows
- selecting a remote-only row and choosing Add prefills catalog name and suggested root path(s)

### `bb completion [bash|zsh|fish|powershell]`

Prints shell completion scripts to stdout.

Examples:

- `bb completion zsh > "${fpath[1]}/_bb"`
- `bb completion bash > ~/.local/share/bash-completion/completions/bb`

## Exit Codes

- `0`: success
- `1`: command completed but found unsyncable state (`scan`, `sync`, `doctor`, `fix` list/apply when still unsyncable)
- `2`: usage error or hard failure

## Configuration

Config file path:

- `~/.config/bb-project/config.yaml`

Default config:

```yaml
version: 1
state_transport:
  mode: external
github:
  owner: your-github-username
  default_visibility: private
  remote_protocol: ssh
clone:
  default_catalog: ""
  shallow: false
  filter: ""
  presets:
    references:
      shallow: true
      filter: blob:none
  catalog_preset:
    references: references
link:
  target_dir: references
  absolute: false
sync:
  auto_discover: true
  include_untracked_as_dirty: true
  default_auto_push_private: true
  default_auto_push_public: false
  fetch_prune: true
  pull_ff_only: true
  scan_freshness_seconds: 60
scheduler:
  interval_minutes: 60
notify:
  enabled: true
  dedupe: true
  throttle_minutes: 60
integrations:
  lumen:
    enabled: true
    show_install_tip: true
    auto_generate_commit_message_when_empty: false
```

Important notes:

- v1 supports only `state_transport.mode: external`.
- `github.owner` is required (`bb init` fails if blank).
- `scheduler.interval_minutes` controls cadence used by `bb scheduler install`.
- set `integrations.lumen.show_install_tip: false` to hide Lumen install/config tips.
- set `integrations.lumen.auto_generate_commit_message_when_empty: true` to run `lumen draft` automatically in commit-producing `bb fix` actions when commit message is empty/`auto`.

## State Layout

Shared (externally synced):

- `~/.config/bb-project/config.yaml`
- `~/.config/bb-project/repos/*.yaml`
- `~/.config/bb-project/machines/*.yaml`

Repo metadata file naming:

- `repos/<repo_key>.yaml` with `/` replaced by `__`
- examples:
  - `software/api` -> `software__api.yaml`
  - `software/openai/codex` -> `software__openai__codex.yaml`

Local runtime state (not required to sync):

- `~/.local/state/bb-project/machine-id`
- `~/.local/state/bb-project/lock`
- `~/.local/state/bb-project/notify-cache.yaml`

Write ownership convention:

- each machine writes only its own `machines/<machine-id>.yaml`
- repo metadata files are shared, low churn, last-writer-wins

## Syncability Rules

A repo is syncable only if all are true:

- has `origin`
- no operation in progress (`merge`, `rebase`, `cherry-pick`, `bisect`)
- no dirty tracked files
- no untracked files when `include_untracked_as_dirty=true`
- current branch has upstream
- branch is not diverged
- if ahead commits exist, push is allowed by policy or `--push`

Unsyncable reasons include:

- `missing_origin`
- `operation_in_progress`
- `dirty_tracked`
- `dirty_untracked`
- `missing_upstream`
- `diverged`
- `push_policy_blocked`
- `push_failed`
- `sync_conflict_requires_manual_resolution`
- `sync_feasibility_probe_failed`
- `pull_failed`
- `checkout_failed`
- `target_path_nonempty_not_repo`
- `target_path_repo_mismatch`
- `clone_required` (non-blocking)
- `catalog_not_mapped` (non-blocking)

## Notification Behavior

When `sync --notify` is used:

- only unsyncable repos are considered
- repos with only non-blocking unsyncable reasons are skipped
- notifications are deduplicated by reason fingerprint per repo cache key
- backend selection priority: `--notify-backend` > `BB_NOTIFY_BACKEND` > `stdout`
- `stdout` backend writes `notify <repo>: <fingerprint>`
- `osascript` backend sends macOS desktop notifications

## Safety Guarantees

- No writes into non-empty non-repo target paths during ensure/sync.
- Existing conflicting target paths are marked unsyncable instead of overwritten.
- Branch switching follows winner only when local repo is syncable.
- No cross-catalog fallback during reconcile (`repo_key` catalog is authoritative).
- Sync does not auto-clone by default (`auto_clone_on_sync` must be enabled per catalog).
- Global per-machine lock prevents concurrent `bb` processes from racing local state writes.

## Practical Workflow

On machine A:

```bash
./bb init api
./bb sync
```

External sync propagates state files.

On machine B:

```bash
./bb sync
./bb status
```

Install periodic scheduler:

```bash
./bb scheduler install
```

## Releases

Versioning and release PR/tag creation is automated with `release-please` on `main`.

Release flow:

1. Merge Conventional Commit messages into `main`.
2. `release-please` opens/updates a release PR with version bump + changelog.
3. Merge that PR to create a `vX.Y.Z` tag and GitHub release.
4. The same `Release Please` workflow runs GoReleaser when a release is created, publishing binaries and updating Homebrew tap formula.

Required GitHub secrets:

- `HOMEBREW_TAP_GITHUB_TOKEN`: PAT with push access to `<org>/homebrew-tap`.

Homebrew install:

```bash
brew tap <org>/tap
brew install bb
```

## Development

Run tests:

```bash
just test
```

Regenerate CLI docs/manpages:

```bash
just docs-cli
```

Run focused e2e suites:

```bash
go test ./internal/e2e -run TestInitCases -count=1
go test ./internal/e2e -run TestSyncBasicCases -count=1
go test ./internal/e2e -run TestSyncEdgeCases -count=1
```

Repository structure:

- `cmd/bb`: CLI entrypoint
- `cmd/bb-docs`: CLI docs/manpage generator
- `internal/cli`: argument parsing and dispatch
- `internal/app`: orchestration and command behavior
- `internal/domain`: core rules and types
- `internal/state`: YAML persistence and lock handling
- `internal/gitx`: git command wrapper
- `internal/e2e`: end-to-end behavior tests

## Test/Debug Environment Variables

Used primarily by test harness:

- `BB_MACHINE_ID`: override machine ID
- `BB_NOW`: override current time (`RFC3339`)
- `BB_TEST_REMOTE_ROOT`: use local bare-repo test backend for `init`

## Current Limitations

- Stale lock recovery is not yet implemented (planned v1.1).
- Sync orchestration code is large and being refactored in v1.1.
- Notification throttle enforcement is planned in v1.1.

## Troubleshooting

### `another bb process holds the lock`

- Check for a currently running `bb` process and wait for completion.
- Current v1 behavior does not recover stale lock files automatically.
- If you are certain no `bb` process is running, remove:
  - `~/.local/state/bb-project/lock`

### `unsupported state_transport.mode`

- Ensure `~/.config/bb-project/config.yaml` contains:
  - `state_transport.mode: external`

### `invalid catalog "<name>"`

- Add catalog first:
  - `bb catalog add <name> <root>`
- Or verify selection:
  - `bb catalog list`
- If the catalog exists on other machines but not locally:
  - run `bb config` and add a local mapping for that catalog

### `init` fails around GitHub repo creation

- Confirm `gh` is installed and authenticated (`gh auth status`).
- Set `github.owner` in `config.yaml`.
- Check whether repo already exists with conflicting ownership/name.

### Repo remains unsyncable

- Run:
  - `bb doctor`
- Typical fixes:
  - commit or discard local changes
  - set upstream for current branch
  - resolve diverged history manually
  - resolve path conflicts at target clone location

## Related Docs

- Spec: `docs/SPEC.md`
- Safe syncing plan/spec: `docs/013-SAFE-SYNCING.md`
- Prompt/build notes: `docs/PROMPT.md`
- v1.1 hardening plan: `docs/PLAN-V1.1.md`
