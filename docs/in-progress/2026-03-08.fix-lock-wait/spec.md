# Spec

## Goal

Fix interactive lock contention UX for `bb fix`:

1. no raw terminal capability garbage after lock contention
2. interactive `bb fix` waits inside TUI instead of failing immediately
3. lock file records why lock exists, so waiting UI can explain owner + elapsed time

## Decisions

### Lock metadata

Extend lock payload with:

- `command`: normalized command label, examples:
  - `sync`
  - `fix`
  - `fix action push`
  - `scan`

Rules:

- all global-lock callsites pass a command/reason string
- parsing old lock files remains supported
- missing/empty command displays as `another bb command`

### Lock API shape

Add state-level primitives:

- acquire lock with metadata
- inspect current lock metadata without acquiring
- typed lock-held error carrying parsed metadata when available

Behavior:

- active lock => return typed error with metadata when readable
- stale lock recovery unchanged
- old/corrupt lock files still supported

### Interactive fix boot flow

Boot model gets explicit phases:

- preparing
- waiting for lock
- loading repositories / checks

Flow:

1. boot starts TUI immediately
2. boot attempts to load interactive fix model
3. if load fails with lock-held error:
   - remain in boot model
   - render wait message using metadata:
     - command owner
     - elapsed time
   - keep polling lock until released
4. when lock clears, finish loading real fix model and transition

### Wait status content

Primary status:

- `Waiting for bb sync to finish...`

Secondary detail:

- `Lock held on <hostname> for 1m12s. Interactive fix opens automatically when startup checks complete.`

Fallbacks:

- unknown command => `Waiting for another bb command to finish...`
- unknown timestamp => omit elapsed duration

### Polling

- modest fixed retry interval; keep implementation simple
- retries only in boot wait state
- quit key still exits immediately

## Validation

Red tests first:

- state:
  - lock file writes command metadata
  - active lock error exposes metadata
  - old payload still parses
- app/tui:
  - boot model stays active on lock-held error and shows wait copy
  - boot model retries and transitions once lock released
  - status text includes command + elapsed duration

Broader:

- targeted `go test` for `internal/state` and `internal/app`
- then broader repo gates if practical within session
