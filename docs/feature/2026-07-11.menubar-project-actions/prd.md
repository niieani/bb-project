## Problem Statement

The macOS menubar identifies repositories needing attention but does not let the user act on an individual repository. The global Sync button provides only a generic “Syncing…” state, leaving the user unable to see which repository is active, what bb is doing, or where a failure occurred.

## Solution

Show compact, contextual Fix and Sync controls on actionable local repository rows. Let bb—not the macOS presentation layer—decide which actions are valid and safe. Stream structured operation progress so the active row shows a spinner and the popup footer shows one concise status line. Refresh live fleet state after every operation and attach failures to the affected repository.

## User Stories

1. As a bb user, I want a Sync control beside a local repository that needs and can safely accept synchronization, so that I can update only that repository.
2. As a bb user, I want no Sync control for a repository where synchronization cannot make safe progress, so that the UI does not offer misleading actions.
3. As a bb user, I want a Fix control beside a repository with one safe deterministic fix, so that I can resolve it without leaving the menubar.
4. As a bb user, I want a Fix… selector when several safe deterministic fixes apply, so that I can choose the intended repair.
5. As a bb user, I want risky or interactive repairs excluded from one-click Fix, so that commits, force pushes, conflict resolution, and destructive changes never happen accidentally.
6. As a bb user, I want remote-machine repository observations to remain read-only, so that this Mac never pretends it can mutate another machine.
7. As a bb user, I want the active repository to show a spinner during global synchronization, so that I can see where work is happening.
8. As a bb user, I want a row action to show a spinner on that same row, so that action ownership is immediately clear.
9. As a bb user, I want a concise status line such as “Fetching origin for bb-project”, so that I understand current progress without raw Git noise.
10. As a bb user, I want global synchronization progress to include completed and total repository counts when known, so that I can judge remaining work.
11. As a bb user, I want competing mutation controls disabled while bb holds its mutation lock, so that operations cannot race.
12. As a bb user, I want successful operations to refresh status automatically, so that resolved repositories disappear or change state promptly.
13. As a bb user, I want failures attributed to the affected repository and summarized in the footer, so that I know what failed and where.
14. As a bb user, I want completed or failed state to remain readable long enough to understand it, so that feedback does not vanish instantly.
15. As a CLI user, I want machine-readable action capabilities, so that all clients use the same eligibility and safety policy.
16. As a CLI client author, I want structured streaming operation events, so that progress can be presented without parsing human logs.
17. As a bb maintainer, I want the existing fix action specifications and risk model reused, so that menubar behavior cannot diverge from `bb fix`.
18. As a bb maintainer, I want targeted sync to use the existing sync engine and global lock, so that its behavior matches global sync.
19. As a bb maintainer, I want human CLI rendering independent from machine events, so that copy changes do not break clients.
20. As a bb user, I want status color, text, controls, and progress to coexist in a compact scrollable popup, so that actionability does not reduce scanability.

## Implementation Decisions

- Go owns action eligibility, safety, labels, stable identifiers, execution, and progress messages.
- Attention/status JSON exposes actions only for the local machine. Swift does not infer actions from reasons, states, colors, or display text.
- Only safe, deterministic, non-interactive fix actions are eligible for one-click execution. Existing fix action specifications and risk classification are authoritative.
- Multiple eligible fixes render through a compact Fix… selection control; one eligible fix renders Fix.
- Targeted synchronization is an explicit CLI capability selecting one repository while retaining the global mutation lock and sync engine.
- Operations provide a stable newline-delimited JSON event stream. Events identify operation kind, repository key when applicable, phase, concise message, completion counts when known, result, and error detail.
- Machine events are distinct from normal human output; the menubar never parses human logs.
- Swift consumes events incrementally through an asynchronous stream and maintains one active mutation, per-repository operation state, the latest status line, and final failure state.
- Mutating controls are disabled while another mutation is active. No implicit queue or cancellation is introduced.
- Operation completion triggers a fresh status and overview load.
- Raw command output and stack traces do not appear in the popup. Errors are concise and actionable.
- Shared fixtures cover the Go-to-Swift status/event contracts.

## Testing Decisions

- Tests assert externally visible contracts and repository effects, not private helper structure.
- Go contract tests verify safe action inclusion, unsafe/interactive exclusion, remote exclusion, stable JSON shape, and empty-array behavior.
- Go end-to-end tests verify targeted sync changes only the selected isolated repository and that global/targeted/fix operations emit ordered attributable events including failures.
- CLI tests verify option forwarding and separation of human and machine output.
- Swift contract tests decode shared Go fixtures.
- Process client tests execute fixture scripts that stream delayed JSON lines, proving incremental delivery rather than completion-time buffering.
- Model/presentation tests verify row controls, active spinners, progress messages, mutation serialization, success refresh, and repository-specific failure display.
- Existing status contract, fix action specification, sync end-to-end, process client, and menu presentation tests provide prior art.
- Final acceptance rebuilds and reloads the local application and validates the real popup through Computer using live bb data.

## Out of Scope

- Running risky, destructive, or interactive fixes directly from the menubar.
- Embedding the full `bb fix` wizard in the popup.
- Mutating repositories reported by other machines.
- Concurrent mutations, operation queues, pause, resume, or cancellation.
- A scrollable raw Git/command log console.
- Redesigning notification policy or the broader fleet status model.

## Further Notes

The popup remains a thin native presentation over bb domain contracts. Status dots continue to be paired with textual descriptions. The single-line progress treatment follows the project’s calm, compact UI rules and avoids the uneven timing and instability of parsing verbose command output.
