# bb

`bb` is a local-first CLI that helps keep Git repositories consistent across multiple machines.

It automates:

- repository bootstrap (`git init`, remote setup, metadata registration)
- discovery of repos under configured catalog roots
- safe cross-machine convergence from `synced` observations
- tiered repository state reporting (`synced`, `pending`, `wip`, `blocked`)

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
- `synced`: clean and eligible for winner selection/convergence
- `pending`: known policy/config remediation is available
- `wip`: normal local work; never makes sync or doctor fail
- `blocked`: requires a human decision and makes sync/doctor exit non-zero

For each `repo_key`, `bb` picks the newest `synced` observation as winner and tries to converge local copies when safe. Machine observations use breaking schema v2; v1 files are skipped with a warning naming the machine that must be upgraded.

## Requirements

- macOS/Linux shell environment
- Go `1.26.0` (for building/testing)
- `just` in `PATH`
- `git` CLI in `PATH`
- `gh` CLI in `PATH` and authenticated (`gh auth login`) for GitHub operations (`bb init`, GitHub create/fork fixes, GitHub push-access checks)
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
- optional `github.preferred_remote_url_template` for custom GitHub remote URL formatting
- sync and attention-policy options
- Lumen integration defaults (install tips and optional AI commit generation when commit message is empty)
- catalogs, per-catalog repository layout depth (`1` or `2`), and default catalog

GitHub CLI prerequisite note:

- the wizard checks whether `gh` is installed and authenticated
- onboarding and the GitHub step both show remediation when missing (install `gh`, run `gh auth login`)

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

- registered project selector (`repo_key`, `catalog:project`, or unique project name)
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

- Resolves registered project metadata first. When input matches a registered project, clone uses the recorded origin URL, catalog, and canonical project path.
- Uses clone defaults from `clone.*` config, then applies catalog preset mapping from `clone.catalog_preset`, then applies explicit CLI flags.
- Registered project clones do not require `clone.default_catalog`; the repo's recorded catalog is authoritative.
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

Behavior:

- Repositories with cached `push_access=unknown` (or unset legacy values) are re-probed during scan, even when the local branch is not ahead.

Exit code is `1` only when at least one observed repo is `blocked`.

### `bb sync [flags]`

Performs full convergence flow:

1. observe local repos
2. publish machine observations
3. load all machine and repo metadata state
4. reconcile by winner
5. pull/checkout and clone only when safe and allowed

Before observation, sync can align a GitHub `origin` URL with `github.preferred_remote_url_template`. It sets the candidate URL, verifies it with bounded `git ls-remote --heads origin`, and keeps it only on success; failure restores the previous URL byte-for-byte and leaves `remote_format_mismatch` as a warning. Disable with `sync.auto_align_remote_format: false`. The `align-remote-format` fix uses the same verified operation.

Flags:

- `--include-catalog <name>` (repeatable)
- `--push` (allow pushing ahead commits when repo policy blocks by default)
- `--dry-run` (observe/reconcile decisions without write-side sync actions)

Additional behavior:

- Reconcile target catalog is strict: `repo_key` catalog must match a local catalog mapping.
- Missing local mapping for a remote-known catalog is skipped with warning (no cross-catalog fallback).
- `--include-catalog` for a catalog known on other machines but missing locally returns a hint to map catalogs via `bb config`.
- Clone during sync is controlled per catalog by `auto_clone_on_sync` (default off).

Exit code is `1` only when selected catalogs still contain `blocked` repos after sync. `pending` and `wip` do not force exit code `1`.

### `bb status [--json] [--include-catalog <name> ...]`

Shows last recorded machine repo state.

- plain mode: one line per repo with tier/reasons, followed by a tier summary
- `--json`: stable programmatic contract for the menubar app and other tooling

The JSON contract exposes:

- `machine_id`
- selected local `repos`, each with `repo_key`, `name`, `catalog`, `path`, `state`, `reasons`, `warnings`, `last_activity_at`, and Go-computed `actions`
- `summary` counts for total/synced/pending/wip/blocked repositories and warnings
- `last_sync`: the local machine's latest `sync_run` journal event, or explicit `null` before the first run
- `attention.items`: fleet-wide non-synced repositories with machine/repository identity, state, reasons, dominant reason, activity time, and Go-computed notification `eligible` status
- `attention.eligible_count`, Go-owned `attention.throttle_minutes`, and a deterministic SHA-256 `attention.fingerprint` covering the eligible fleet attention set
- `source_warnings`: explicit fleet-state loading warnings, including machines publishing an obsolete schema

Consumers must use `eligible` rather than reproduce quiet-period, stale-WIP, or tier policy. Pending and recently active blocked repositories remain visible in the snapshot but are ineligible. Shared contract fixtures live in `fixtures/status/` and are decoded by both Go and Swift tests.

### Native macOS menubar app

The dependency-free Swift app lives in `macos/BBMenuBar/`. It is intentionally menu-bar-only (no Dock icon) and treats the installed `bb` CLI JSON as its sole data source; it does not read state files, inspect Git, or implement sync policy. The title shows `✅` with the local repository count when the Go-provided fleet attention count is zero, `⚠️` with the eligible attention count otherwise, and an explicit `⛔ Error` state when `bb` is missing or returns an incompatible/malformed contract. Emoji preserve the intended color semantics even when macOS applies template rendering to menu-bar symbols.

For local development:

```bash
swift test --package-path macos/BBMenuBar
./script/build_and_run.sh --verify
```

The Codex Run action uses the same build-and-run script. Development launches pass the just-built repository `bb` binary explicitly; installed builds resolve `bb` from `PATH`, Homebrew, or `~/bin`.

Opening the status item shows compact sections for local blocked repositories, stale local WIP, actionable local repositories, and eligible attention on other machines. Empty sections disappear. Go-owned action capabilities expose `Sync` only where synchronization can safely make progress and `Fix`/`Fix…` for deterministic non-interactive remedies; risky and remote actions remain unavailable. Row mutations stream the active repository and concise phase into the popup; competing mutations remain disabled and completion refreshes status plus overview. The app refreshes every five minutes and after macOS wake.

Automation clients can run one repository with `bb sync --repo <repo-key> --events-json`. The JSON-lines stream reports ordered operation/repository lifecycle, phase, message, result, error, and completed/total repository counts without requiring clients to parse human logs. Global sync uses the same stream, allowing clients to attribute live progress and failures to each repository.

Machine clients may execute a status-advertised safe fix with `bb fix <repo-key> <action-id> --events-json --no-refresh`. Event mode rejects actions not explicitly classified safe for machine clients.

The app is also the sole notification surface. It requests macOS notification permission explicitly, submits one fleet digest from the Go-computed eligible attention snapshot, persists whole-set fingerprint deduplication and throttling, and shows permission, persistence, or delivery failures in the menu. Clicking a digest opens its exact repository attention list. The distributed app registers itself with macOS launch-at-login; approval or registration failures remain visible rather than silently disabling background delivery.

Install and update the signed, notarized app through Homebrew; its cask installs `bb` as a dependency:

```bash
brew install --cask niieani/tap/bb-menubar
brew upgrade --cask bb-menubar
open -a BBMenuBar
```

Release, signing, and Gatekeeper verification details are in `docs/releasing-macos-menubar.md`.

### `bb overview [--all] [--json] [--include-catalog <name> ...]`

Shows a read-only cross-machine matrix with the local machine first. Non-synced cells include the dominant reason and last-activity age; missing copies show as not cloned. Repositories synced everywhere collapse into one count unless `--all` is used. Stale machine publications are called out explicitly. `--json` is a stable full-matrix contract for tooling and always includes collapsed repositories, every machine, states, reasons, warnings, and timestamps. Overview always exits `0`, including when blocked repositories exist.

In JSON, machines expose `id`, `here`, `published`, optional `updated_at`, and `stale`; unpublished local columns omit `updated_at` rather than fabricating an age. Repository rows expose `repo_key`, `synced_everywhere`, and ordered `cells`; each cell exposes machine identity, presence, optional state, reasons, warnings, and last activity.

### `bb log [--repo <selector>] [--machine <id>] [--limit N] [--json]`

Shows the merged newest-first fleet sync journal. It records sync summaries, convergence, clones, pushes, verified remote alignment or rollback, and applied fix actions. Per-machine JSONL journals live in the shared state directory and prune to `journal.max_entries`. Journal write failures are logged without changing sync or fix outcomes.

### `bb doctor [--include-catalog <name> ...]`

Prints non-synced tiers, reasons, and warnings from the machine file.

- refreshes local observations only when the last scan snapshot is stale (default threshold: 60 seconds; configurable via `sync.scan_freshness_seconds`)
- when GitHub is configured or selected repos use GitHub remotes, also reports warnings if `gh` is missing or not authenticated, with remediation commands

Returns `1` only if a blocked repo is present in selected catalogs.

### `bb ensure [--include-catalog <name> ...]`

Alias for sync convergence (`bb sync` with include filters).

### `bb scheduler`

Manage macOS launchd scheduling for periodic sync.

- `bb scheduler install`
  - installs/replaces a LaunchAgent that runs `bb sync --quiet`
  - reads `scheduler.interval_minutes` from config
- `bb scheduler status`
  - reports whether LaunchAgent is installed and its current interval
- `bb scheduler remove`
  - unloads and removes the LaunchAgent

### `bb fix [project] [action] [flags]`

Inspect repositories and apply context-aware fixes.

Forms:

- `bb fix` opens interactive table mode (requires interactive terminal).
- `bb fix <project>` prints repo state and currently eligible fixes.
- `bb fix <project> <action>` applies one action and re-observes state.

Interactive apply behavior:

- Risky fixes (`push`, `sync-with-upstream`, `set-upstream-push`, `stage-commit-push`, `stash`, `create-project`) open a confirmation wizard before execution.
- Wizard shows changed files with `+/-` stats, target branch context, and a per-repo skip option.
- For commit-producing actions, wizard includes commit message input with symbolic `✨` generation (Lumen draft).
- `create-project` wizard includes `Stage & commit before initial push` toggle (enabled by default) so local unstaged/uncommitted files can be included in the first remote push.
- `stash` wizard includes `Stash mode` (`Staged + unstaged` or `Staged only`) and stash-name input with symbolic `✨` generation (Lumen draft).
- When `Publish as new branch (optional)` is set, `bb fix` creates and switches to that branch before staging/committing, so the original branch ref is left unchanged.
- When changed files are shown, press `⌥V` on macOS (or `alt+v` on other platforms) to launch Lumen visual diff and return to the same wizard state.
- Wizard can generate a minimal root `.gitignore` when missing.
- Wizard summary shows commits created by each applied step (short SHA + commit subject), including auto-generated commit messages.
- In list mode, when repository details wrap (for example long paths or action-help text), `bb fix` shrinks the table viewport first so top chrome and footer help remain visible without truncating details text, and keeps one-row navigation stable (no sudden page jump when moving by one row).
- In list mode, the primary panel uses a compact titled border (`bb fix · Interactive remediation by repository state`) to preserve vertical space, and selected-repo metadata is rendered on one dot-separated line.
- Busy list-mode states (for example `r` revalidation) now recolor the full primary-panel border consistently, including the titled top edge.
- The list-mode summary stats row is responsive: it renders pill boxes when they fit, and on narrow terminals it keeps the same uppercase metric chips/order/colors but drops borders and wraps by chip to prevent horizontal overflow artifacts.
- Selected-repo metadata wrapping is segment-aware (wraps at ` · ` boundaries), so labels stay attached to their values and avoid orphan trailing tokens on separate lines.
- In list mode, `bb fix` keeps the primary panel top-anchored and places footer help immediately below it (no artificial spacer gap between panel and footer); available height is absorbed by list sizing.
- In list mode, `enter` runs currently selected fixes; when none are selected, it runs the currently browsed fix for the selected repo.
- In list mode, `i` toggles session ignore for the selected repo (ignore/unignore).
- Interactive list ordering is globally `blocked`, stale `wip`, `pending`, fresh `wip`, `synced`, then `ignored`; catalogs (default first) order repositories within each tier. `clone_required` appears in the pending group.
- Before computing fix eligibility, `bb fix` re-probes repositories whose cached `push_access` is `unknown`.
- Targeted non-interactive `bb fix <project> [action]` computes risk checks and unknown push-access probes only for the selected repository.
- Non-TUI `bb fix` execution passes through git stdio for synchronous git commands (for example interactive authentication prompts).
- When immediate apply fails in interactive mode, the error banner surfaces concrete command failure details (first failure + summary), not just a generic failure message.
- For GitHub origins (including `*.github.com` aliases), the probe treats `gh` viewer permission as authoritative when available; it falls back to `git push --dry-run` only when `gh` cannot determine access.
- Repositories that still have `push_access=unknown` after probing do not get push-related fix actions; run `bb repo access-refresh <repo>` after resolving probe blockers.
- The startup loading screen shows phase-based progress and collapses noisy multiline probe/auth errors into concise status text while checks continue.
- If another `bb` command currently holds the global lock, interactive `bb fix` stays in its loading screen, shows which command is running plus lock age/host, and opens automatically once the lock is released.

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
- `stash`
- `pull-ff-only`
- `set-upstream-push`
- `enable-auto-push`
- `move-to-catalog`
- `align-remote-format`
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

### `bb repo move <repo> --catalog <target> [flags]`

Moves a managed repository to another catalog and updates shared metadata so other machines can converge.

Flags:

- `--catalog <name>` (required)
- `--as <catalog-relative-path>`
- `--dry-run`
- `--no-hooks`

Behavior:

- Resolves `<repo>` using existing local selector rules.
- Validates destination path safety before applying.
- Moves local directory to the new catalog path.
- Rewrites metadata to the new `repo_key` and records old key history.
- On other machines, stale local paths surface as non-blocking `catalog_mismatch` and can be remediated with `bb fix` action `move-to-catalog`.
- Runs `move.post_hooks` unless `--no-hooks`.

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
- `1`: command completed but found blocked state (`scan`, `sync`, `doctor`, `fix` list/apply when still blocked)
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
  preferred_remote_url_template: ""
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
move:
  post_hooks: []
sync:
  auto_align_remote_format: true
  auto_discover: true
  include_untracked_as_dirty: true
  default_auto_push_private: true
  default_auto_push_public: false
  fetch_prune: true
  pull_ff_only: true
  scan_freshness_seconds: 60
scheduler:
  interval_minutes: 60
attention:
  throttle_minutes: 60
  quiet_hours: 2
  wip_stale_hours: 24
overview:
  machine_stale_days: 3
journal:
  max_entries: 500
integrations:
  lumen:
    enabled: true
    show_install_tip: true
    auto_generate_commit_message_when_empty: false
```

Important notes:

- v1 supports only `state_transport.mode: external`.
- `github.owner` is required (`bb init` fails if blank).
- `github.preferred_remote_url_template` is optional; when set it overrides `github.remote_protocol` for GitHub URLs.
- Template placeholders: `${org}` (alias `${owner}`) and `${repo}`.
- `scheduler.interval_minutes` controls cadence used by `bb scheduler install`.
- `attention.quiet_hours` and `attention.wip_stale_hours` control Go eligibility policy; `attention.throttle_minutes` is exported to the native app for delivery throttling.
- The removed `notify:` section is rejected explicitly. Rename it to `attention:` and remove `enabled`; there is no legacy alias or automatic migration.
- `move.post_hooks` run after a successful repository move (`bb repo move` and `bb fix ... move-to-catalog`) on each machine where the move executes.
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
- `~/.local/state/bb-project/lock` (`pid`, `hostname`, `created_at`, `command`)

Write ownership convention:

- each machine writes only its own `machines/<machine-id>.yaml`
- repo metadata files are shared, low churn, last-writer-wins

## Repository State Rules

- `wip`: `missing_origin`, `operation_in_progress`, dirty files, `missing_upstream`, or `push_policy_blocked`.
- `pending`: `clone_required`, `catalog_not_mapped`, or `catalog_mismatch`.
- `blocked`: divergence, push-access/action failures, sync conflicts/probe failures, or target-path conflicts.
- `synced`: no state reasons. Only this tier participates in winner selection or convergence writes.

Precedence is `blocked` > `pending` > `wip` > `synced`. Unknown reasons fail explicitly until assigned a tier. `last_activity_at` records the newest Git HEAD/index/dirty-path mtime but is excluded from `state_hash`, preventing observation churn.

## Notification Behavior

The installed `BBMenuBar.app` owns native delivery through UserNotifications:

- Go marks blocked repositories outside the quiet period and WIP older than the stale threshold as eligible; pending and recently active blocked repositories remain in the snapshot but do not alert.
- One digest covers eligible repositories across the fleet and lists at most four repositories plus an overflow count.
- An unchanged whole-attention fingerprint never repeats. A changed fingerprint waits for the Go-provided throttle interval, and an empty set resets deduplication.
- Permission denial/unavailability, persistence errors, and delivery failures render explicitly in the menu.
- Clicking a notification opens the digest's relevant repository list.
- The app registers for launch at login so periodic/wake refresh can deliver while no terminal is open.

## Safety Guarantees

- No writes into non-empty non-repo target paths during ensure/sync.
- Existing conflicting target paths are marked blocked instead of overwritten.
- Branch switching follows winner only when local repo is synced.
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
4. `Release Please` then calls the shared publish workflow, which runs GoReleaser plus the native app build, Developer ID signing, notarization, release-asset upload, and Homebrew tap cask updates.
5. The standalone `Release` workflow is a manual recovery path for republishing the menubar app for an existing tag. It skips GoReleaser so existing CLI assets cannot block app recovery.

The app release suite runs with a constrained Swift cooperative executor so blocking process I/O cannot be masked by a high-core development machine.

Required GitHub secret:

- `OP_SERVICE_ACCOUNT_TOKEN`: 1Password service account token with read access to the Apple release-signing items.

Required 1Password references:

- `op://Automation/Apple Developer App Store Connect AuthKey Github Releases/AuthKey.p8`
- `op://Automation/Apple Developer App Store Connect AuthKey Github Releases/NOTARY_ISSUER_ID`
- `op://Automation/Apple Developer App Store Connect AuthKey Github Releases/NOTARY_KEY_ID`
- `op://Automation/Apple Release Signing Developer ID Certificate/Apple Release Signing Developer ID Certificate.p12`
- `op://Automation/Apple Release Signing Developer ID Certificate/password`
- `op://Automation/GitHub Token for homebrew-tap/token`

Homebrew install:

```bash
brew tap <org>/tap
brew install --cask bb
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

- Sync orchestration code is large and being refactored in v1.1.

## Troubleshooting

### `another bb process holds the lock`

- Non-interactive commands fail fast; interactive `bb fix` waits in its loading screen until the lock is released.
- Recent lock files include the owning command, host, and creation time, so contention messages can identify what is running.
- Stale lock files are recovered automatically when the recorded process is gone or the lock ages out.
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

### Repo remains blocked

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
