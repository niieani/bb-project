# Signal repair foundation

## Brief

Implement `BB-umgmqrjf`, completing F1–F5 before either nested PRD. Replace binary syncability with tiered state, schema v2, verified remote alignment, quiet aggregated notifications, tiered fix/status/doctor language, then execute the guarded fleet rollout.

## Goal

`bb` classifies repositories as synced, pending, wip, or blocked; records schema-v2 observations and activity; treats remote-format drift as a verified-fix warning; alerts only on the aggregate activity-aware attention set; exposes consistent tier language; and safely rolls the released schema across both machines.

## Scope

- Read/write: implementation, tests, README and related docs, fp issues/comments, atomic commits, release/fleet operations required by F5.
- In scope now: F1–F5 in dependency order.
- Deferred until F1–F5 complete: nested overview and journal PRDs.
- Out: automatic WIP synchronization, winner redesign, unrelated cleanup.

## Principles

- Breaking cleanup; no compatibility wrappers or v1 migration.
- Explicit failures for unknown tiers and required values.
- External behavior tests first; parallel isolated Go tests.
- Preserve convergence write safety: only synced observations win or mutate.
- Never expose the shared fleet to dev schema v2 before the F5 release rollout.

## Completion criteria and verification

- Each F1–F5 acceptance criterion passes its focused tests and project full suite.
- Per slice: red test evidence, formatting, lint/build/full tests, atomic semantic commit, fp revision link, completion comment, done status.
- User-facing docs reflect shipped behavior.
- F5: released v2 installed on both machines; v1 files absent; sync/doctor evidence clean.
- Independent final review reports no unresolved blocking findings.

## Execution shape

Five sequential TDD slices followed by independent PRD review. One workdesk for the entire parent PRD and later nested PRDs only after foundation completion.
