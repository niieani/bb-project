# bb Project Sync Specification

## Motivation

Working across multiple macOS computers creates repetitive and error-prone repository management work:

- New projects require repeated setup (directory, git init, GitHub repo creation, remote wiring).
- Existing projects drift between machines (missing clones, different checked-out branches).
- Local unsynced work (dirty files, unpushed commits, diverged branches) blocks safe automation.

The goal of `bb` is to reduce manual labor while staying safe:

- Auto-discover and track repositories under configured local catalogs.
- Keep repositories present on every machine.
- Converge branch selection between machines only when local state is syncable.
- Report unsyncable state clearly and notify when needed.

## General Description

`bb` is a local-first CLI for repository bootstrap and cross-machine convergence.

### Canonical state transport

State is synchronized by an external tool chosen by the user (for example Syncthing, Dropbox, iCloud Drive, rsync, etc.).

- Config keeps `state_transport.mode` for forward compatibility.
- Only `external` mode is implemented in v1.

### Catalogs

A catalog is a named local root directory that may contain git projects.

- Example catalog `software` -> `/Volumes/Projects/Software`
- Example catalog `references` -> `/Volumes/Projects/SoftwareReferences`
- Each machine defines its own catalog root paths in its own machine file.
- One catalog is marked as `default_catalog`.
- `bb init [project]` uses `default_catalog` unless overridden with `--catalog <name>`.
- `bb sync` operates on all catalogs by default, or filtered with `--include-catalog <name>` (repeatable).

### High-level behaviors

1. `bb init [project]`
   - Resolves a target project directory:
     - with `project`: `<catalog_root>/<project>`
     - without `project`: inferred from current working directory when it is inside a catalog project subtree
   - Uses `git` CLI to initialize the repository.
   - Uses `gh` CLI to create a GitHub repo (private by default, personal owner, `--public` supported).
   - Uses `git` CLI to set `origin`.
   - May auto-push current branch when push policy allows (or `--push` is provided).
   - Is idempotent: existing git/GitHub setup is detected and skipped when already correct.
   - Registers repo metadata and observed machine state.

2. Automatic discovery
   - Any new git repo under selected catalogs is auto-added.
   - No manual approval prompt is required.

3. Periodic sync
   - Each machine writes its own observed repo state.
   - Each machine reads all other machines' observed repo states.
   - For each repo, newest syncable observed state is selected as winner.
   - Branch switching follows winner only when local repo is syncable.
   - Unsyncable repos are never auto-switched.

4. Push policy
   - Repo-level `auto_push` policy decides whether ahead commits may be auto-pushed.
   - Default is derived from visibility when repo metadata is first created:
     - private -> `auto_push: true`
     - public -> `auto_push: false`
   - Policy is editable later.

5. Scheduler
   - macOS `launchd` runs `bb sync --notify` periodically.
   - Notifications are deduplicated by fingerprint until state changes.

6. Safe path handling
   - During `bb sync`/`bb ensure`, `bb` never writes into a non-empty folder unless it is the expected git repository.
   - Empty existing target folder is allowed for clone.
   - During `bb init`, initializing git in an existing non-empty project directory is allowed.
   - Path conflicts are marked unsyncable and require manual resolution.

## Terminology

- `repo_id`: canonical repository identifier derived from normalized `origin` URL.
- `catalog`: named root directory containing projects.
- `default_catalog`: catalog used when command does not specify one.
- `syncable`: repository state is safe for automation.
- `unsyncable`: repository state requires manual intervention before automation continues.
- `observed state`: normalized snapshot of sync-relevant repo fields.
- `state_hash`: hash of observed state fields.
- `observed_at`: timestamp when `state_hash` last changed.
- `newest syncable observed state`: syncable record with greatest `observed_at` (tie-break by `machine_id`).
- `winner`: newest syncable observed state selected for a repo.
- `machine_id`: stable per-machine identity.
- `target path conflict`: local filesystem path exists but cannot be safely used for this repo.

## Goals

- Zero-friction repo onboarding and discovery.
- Deterministic branch convergence.
- Minimal shared-state conflicts.
- Clear failure reporting for unsyncable repositories.

## Non-goals

- Auto-resolving diverged git histories.
- Auto-stashing or auto-committing dirty/untracked work.
- Replacing standard Git and GitHub workflows.

## Filesystem Layout

### Shared state directory (externally synchronized)

- `/Users/<user>/.config/bb-project/config.yaml`
- `/Users/<user>/.config/bb-project/repos/<repo-id>.yaml`
- `/Users/<user>/.config/bb-project/machines/<machine-id>.yaml`

Write ownership:

- Each machine writes only its own `machines/<machine-id>.yaml`.
- Repo metadata files are low-churn shared files; concurrent creation uses last-writer-wins.

### Local runtime state (not required to be synchronized)

- `/Users/<user>/.local/state/bb-project/machine-id`
- `/Users/<user>/.local/state/bb-project/lock`
- `/Users/<user>/.local/state/bb-project/notify-cache.yaml`

## Schema

All YAML schemas are versioned with `version: 1`.

### 1) `config.yaml` (shared defaults)

```yaml
version: 1
state_transport:
  mode: external # only mode implemented in v1
github:
  owner: your-github-username
  default_visibility: private
  remote_protocol: ssh # ssh | https
sync:
  auto_discover: true
  include_untracked_as_dirty: true
  default_auto_push_private: true
  default_auto_push_public: false
  fetch_prune: true
  pull_ff_only: true
notify:
  enabled: true
  dedupe: true
  throttle_minutes: 60
```

### 2) `repos/<repo-id>.yaml` (shared repo metadata + policy)

```yaml
version: 1
repo_id: github.com/you/bb-project
name: bb-project
origin_url: git@github.com:you/bb-project.git
visibility: private # private | public | unknown
preferred_catalog: software
auto_push: true # default derived from visibility; editable override
branch_follow_enabled: true
```

### 3) `machines/<machine-id>.yaml` (per-machine config + observations)

```yaml
version: 1
machine_id: mbp14-2025
hostname: mbp14
default_catalog: software
catalogs:
  - name: software
    root: /Volumes/Projects/Software
  - name: references
    root: /Volumes/Projects/SoftwareReferences
updated_at: "2026-02-13T20:31:00Z"
repos:
  - repo_id: github.com/you/bb-project
    name: bb-project
    catalog: software
    path: /Volumes/Projects/Software/bb-project
    origin_url: git@github.com:you/bb-project.git
    branch: main
    head_sha: 0123456789abcdef0123456789abcdef01234567
    upstream: origin/main
    remote_head_sha: 0123456789abcdef0123456789abcdef01234567
    ahead: 0
    behind: 0
    diverged: false
    has_dirty_tracked: false
    has_untracked: false
    operation_in_progress: none # none | merge | rebase | cherry-pick | bisect
    syncable: true
    unsyncable_reasons: []
    state_hash: "sha256:8d95..."
    observed_at: "2026-02-13T20:31:00Z"
```

### 4) `notify-cache.yaml` (local runtime state)

```yaml
version: 1
last_sent:
  github.com/you/bb-project:
    fingerprint: "dirty+ahead"
    sent_at: "2026-02-13T20:35:00Z"
```

## Syncable vs Unsyncable Rules

A repo is `syncable` only if all conditions are true:

1. Repo exists and has valid `origin`.
2. No operation in progress (`merge`, `rebase`, `cherry-pick`, `bisect`).
3. No tracked changes.
4. No untracked files (per config).
5. Current branch has upstream.
6. Branch is not diverged.
7. If `ahead > 0`, push is allowed by policy (`auto_push == true`) or by command flag `--push`.

Unsyncable reasons include:

- `missing_origin`
- `operation_in_progress`
- `dirty_tracked`
- `dirty_untracked`
- `missing_upstream`
- `diverged`
- `push_policy_blocked`
- `push_failed`
- `pull_failed`
- `checkout_failed`
- `target_path_nonempty_not_repo`
- `target_path_repo_mismatch`
- `duplicate_local_repo_id`

## Definition: Newest Syncable Observed State

Observed state fields used for `state_hash`:

- `branch`, `head_sha`, `upstream`, `remote_head_sha`
- `ahead`, `behind`, `diverged`
- `has_dirty_tracked`, `has_untracked`
- `operation_in_progress`
- `syncable`, `unsyncable_reasons`

Publishing rule:

1. On every sync, compute candidate `state_hash`.
2. Compare with last published hash for this repo in this machine file.
3. If unchanged, keep existing `observed_at`.
4. If changed, set `observed_at` to now and store new hash.

Therefore, "newest" means newest meaningful state change, not newest scan tick.

## Deterministic Winner Selection

For each `repo_id`:

1. Gather records from all machine files.
2. Filter `syncable == true`.
3. Pick max `observed_at`.
4. Tie-break by lexicographically smallest `machine_id`.

Winner represents the branch/head other machines should follow when safe.

## Sync Algorithm

Pseudocode for `bb sync [--push] [--notify] [--include-catalog <name>...]`:

```text
acquire_global_lock()
load shared config
ensure state_transport.mode == external
ensure machine_id exists
load this machine file (catalogs + default_catalog + prior observed state)

selected_catalogs =
  --include-catalog values if provided
  else all catalogs from this machine file

auto_discover_repos_under(selected_catalog_roots)
for each local repo in selected catalogs:
  resolve repo_id from normalized origin URL
  ensure repos/<repo-id>.yaml exists
  if repo metadata created now:
    set preferred_catalog from discovered catalog
    set auto_push default from visibility

  collect local git state
  git fetch --prune (if enabled)

  evaluate syncable (policy + CLI flags)
  if syncable:
    if behind > 0 and ahead == 0:
      git pull --ff-only
    if ahead > 0:
      if auto_push true or --push:
        git push
      else:
        mark unsyncable push_policy_blocked

  refresh local git state
  recompute syncable + reasons
  recompute state_hash
  update observed_at only when state_hash changed

write this machine file
read all machines/*.yaml

for each repo_id with shared metadata:
  if repo preferred_catalog not in selected_catalogs:
    continue
  winner = newest syncable observed state
  if no winner:
    continue

  target_catalog =
    repo preferred_catalog if present on this machine
    else default_catalog
  target_path = <target_catalog root>/<repo name>
  if target_path exists and is not empty:
    if target_path is not git repository:
      mark repo unsyncable target_path_nonempty_not_repo
      continue
    if target_path git origin does not match repo_id:
      mark repo unsyncable target_path_repo_mismatch
      continue

  local_matches = local repos with same repo_id in selected catalogs
  if count(local_matches) > 1:
    mark repo unsyncable duplicate_local_repo_id
    continue

  if count(local_matches) == 1:
    local_path = local_matches[0]
    if local repo is syncable:
      if local branch != winner.branch:
        checkout winner.branch (create local tracking branch if required)
      fetch + pull --ff-only
    else:
      leave unchanged
  else:
    if target_path does not exist:
      clone winner.origin_url into target_path
      checkout winner.branch
    else if target_path exists and is empty directory:
      clone winner.origin_url into target_path
      checkout winner.branch
    else:
      adopt existing local repo at target_path

refresh local observations and rewrite this machine file
if --notify:
  notify on newly unsyncable fingerprints (deduped)
release lock
```

## Bootstrap Flow

`bb init [project] [--catalog <name>] [--public] [--push]`:

1. Resolve target catalog (`--catalog` or `default_catalog`).
2. Resolve target project directory:
   - if `project` provided: `<catalog root>/<project>`
   - else infer from cwd by walking upward to the first directory directly under a catalog root.
3. Ensure target directory exists.
4. If target has no git repo, run `git init -b main`; else keep existing repo.
5. Determine remote state:
   - if `origin` already points to expected GitHub repo, skip creation
   - if `origin` missing, run `gh repo create` and set `origin`
   - if `origin` exists but conflicts with requested repo identity, fail with clear error
6. Visibility behavior:
   - default: create private repo
   - `--public`: create public repo
7. Create/update `repos/<repo-id>.yaml` with policy defaults from visibility:
   - private -> `auto_push: true`
   - public -> `auto_push: false`
8. If current branch has commits and no upstream:
   - push with `-u origin <branch>` when `auto_push == true` or `--push` provided
   - otherwise leave unpushed (for example public repo without `--push`)
9. Scan and publish machine state.
10. Re-running `bb init` must be safe and produce no destructive changes.

## Commands (v1)

- `bb init [project] [--catalog <name>] [--public] [--push] [--https]`
- `bb sync [--push] [--notify] [--dry-run] [--include-catalog <name> ...]`
- `bb status [--json] [--include-catalog <name> ...]`
- `bb doctor [--include-catalog <name> ...]`
- `bb scan [--include-catalog <name> ...]`
- `bb ensure [--include-catalog <name> ...]`
- `bb repo policy <repo> --auto-push=<true|false>`
- `bb catalog add <name> <root>`
- `bb catalog rm <name>`
- `bb catalog default <name>`
- `bb catalog list`

## Initial Adoption and Pre-existing Repositories

This section defines behavior when `bb` is first enabled on machines that already contain repositories.

1. First sync on each machine performs discovery first, then publishes observed state.
2. If the same repo already exists on multiple machines, no clone is attempted; each machine adopts its local copy via `repo_id` matching.
3. If a machine has the repo at a different local path/catalog than other machines, that is allowed unless target path conflict rules apply.
4. If clone/ensure target path exists and is empty, clone is allowed.
5. If target path exists and has files but is not a git repo, mark `target_path_nonempty_not_repo` and skip all sync changes for that repo on that machine.
6. If target path is a git repo but not the expected `repo_id`, mark `target_path_repo_mismatch` and skip all sync changes for that repo on that machine.
7. If more than one local path on the same machine maps to the same `repo_id`, mark `duplicate_local_repo_id` and skip changes for that repo until user resolves duplicates.

## Failure Handling

- Never discard local changes.
- Never auto-merge diverged histories.
- Continue processing other repos when one repo fails.
- Return non-zero when any repo remains unsyncable or command-level errors occur.

Suggested exit codes:

- `0`: all processed repos syncable and converged
- `1`: at least one processed repo unsyncable
- `2`: runtime/config/lock error

## Notifications

`bb sync --notify` sends macOS notifications only when:

1. Repo becomes newly unsyncable, or
2. Unsyncable reason fingerprint changes.

Notification payload:

- repo name
- unsyncable reasons (short)
- suggested action

## Example Workflow

1. Machine A checks out `feature/x` in repo `api` (catalog `software`) and remains clean.
2. A runs sync and publishes changed observed state (`branch=feature/x`).
3. External sync tool replicates state files.
4. Machine B runs sync.
5. If B copy of `api` is syncable, B switches to `feature/x` and fast-forwards.
6. If B is dirty/unsyncable, B does not switch and reports reasons.
7. User resolves B state (for example commit+push).
8. B publishes new changed observed state; on next run A may follow B if B is newest syncable state.

## Open Questions (implementation phase)

1. Whether nested git repos under a catalog root are allowed or ignored.
2. Whether `bb init` should create an initial commit by default.
3. How visibility should be refreshed (periodic `gh` check vs cached metadata).
