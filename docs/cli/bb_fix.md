## bb fix

Inspect repositories and apply context-aware fixes.

```
bb fix [project] [action] [flags]
```

### Options

```
      --ai-message                    Generate commit message with Lumen for commit-producing fix actions.
  -h, --help                          help for fix
      --include-catalog stringArray   Limit scope to selected catalogs (repeatable).
      --message string                Commit message for stage-commit-push/publish-new-branch/checkpoint-then-sync actions (or 'auto' for configured empty-message behavior).
      --no-refresh                    Use current machine snapshot without running a refresh scan first.
      --publish-branch string         Target branch name for publish-new-branch or optional publish-to-new-branch flows.
      --return-to-original-sync       After publish-new-branch, switch back to the original branch and run pull --ff-only.
      --sync-strategy string          Sync strategy for sync-with-upstream and pre-push validation (rebase|merge). (default "rebase")
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### Interactive mode notes

- `bb fix` list mode renders a compact bordered title (`bb fix · Interactive remediation for unsyncable repositories`) and a compact selected-repo details line to reduce vertical space usage.
- Selected-repo metadata wraps on segment separators (` · `) so labels stay attached to values.
- In list mode, `enter` runs selected fixes; when none are selected, it runs the currently browsed fix for the selected repo.
- List ordering is by catalog (default first), then state tier: `fixable`, `unsyncable`, `not cloned`, `syncable`, `ignored`.
- Repositories marked with `clone_required` are shown as `not cloned`.
- Before computing fix eligibility, `bb fix` re-probes repositories whose cached `push_access` is `unknown`.
- For GitHub origins (including `*.github.com` aliases), the probe checks `gh` viewer permission first and then validates with `git push --dry-run` as needed.
- Repositories that still have `push_access=unknown` after probing do not get push-related fix actions; run `bb repo access-refresh <repo>` after resolving probe blockers.

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.
