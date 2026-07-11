# Status contract and native menubar

## Brief

Implement fp PRD BB-ztthysfs and every child issue. Extend `bb status --json` into the stable fleet-attention contract, add a native same-repository macOS menubar app, integrate distribution, move macOS notifications into that app, and delete the legacy Go notification delivery paths.

Context: completed signal-repair foundation BB-umgmqrjf and `docs/in-progress/2026-07-08.signal-repair/spec.md`. The PRD's later comment supersedes its original notification out-of-scope statement: M5 is in scope.

## Final behavior

- Go owns state tiers, attention eligibility, dominant reasons, fingerprint, quiet/stale thresholds, and throttle configuration.
- `bb status --json` publishes repository state, summary, last sync, and deterministic eligible attention data as a documented stable contract with shared fixtures.
- Native Swift menu-bar-only app presents CLI JSON, refreshes periodically and after wake, runs detached sync on request, shows explicit source failures, and delivers/deduplicates native fleet notifications.
- Swift reads no bb state files, runs no git operations, and contains no duplicated attention policy.
- Distributed app is tested, signed, notarized, installed/upgraded through Homebrew, launches at login, and reuses existing release credentials/workflow.
- Legacy Go notifier backends, flags, environment variable, scheduler arguments, cache, warnings, tests, and docs do not remain.

## Boundaries

In scope: BB-qiqmeetv, BB-qdcoxizm, BB-oqeftwks, BB-krezgwel, BB-gcpevkel; code, tests, fixtures, release workflow, generated CLI docs, README, and generalized UI/architecture rules.

Out of scope: remediation UI beyond Sync now; Windows/Linux tray ports; App Store distribution; Swift reimplementation of repository/attention policy.

Read: entire repository, fp issue context, referenced local vendor/docs, relevant current official platform/release documentation. Write: repository files, fp status/comments/revision assignments, atomic semantic commits, release/install verification artifacts under this workdesk or `temp.local/2026-07-10/`. No branches. Push only as required by repository session completion instructions.

## Criteria and verification

1. M1 contract and fixtures satisfy all issue state mixes and boundaries. Verify focused Go/e2e golden tests, `just test`, `just build`, JSON inspection.
2. M2 title and explicit failures work from real CLI output. Verify Swift decoding/view-model tests, build/launch, Computer Use inspection.
3. M3 sections, refresh, wake, and detached sync work. Verify Swift tests plus live menu interaction and process/log evidence.
4. M4 produces the app through existing signing/notarization and Homebrew path without parallel credentials. Verify workflow/config tests, local artifact inspection, real-machine install/upgrade where credentials/release context permits.
5. M5 native notifications implement permission/error/dedupe/throttle/click routing and launch at login; Go legacy delivery is absent. Verify Swift seam tests, Go search/tests, scheduler output, signed-app manual notification flow with Computer Use.
6. Every child gets full repository-native gates, fp completion report, done status, atomic semantic commit, and revision assignment.
7. Independent final review finds no unresolved blocker. Verify review report and focused re-review after any fixes.
8. README and relevant permanent docs describe stable contracts, installation, behavior, and notification ownership. Generated CLI/man docs updated only via `just docs-cli`.
9. Final repository gates pass and main is committed, pushed, and synchronized with origin.

## Execution shape

Five dependency-ordered TDD slices. Red external-behavior tests first. Full suite/build before each issue completion. macOS UI requires live acceptance via build/run tooling and Computer Use. Independent review at slice validation/cleanup where required by builder workflow and mandatory final PRD audit.
