## bb fix

Inspect repositories and apply context-aware fixes.

```
bb fix [project] [action] [flags]
```

### Options

```
  -h, --help                          help for fix
      --include-catalog stringArray   Limit scope to selected catalogs (repeatable).
      --message string                Commit message for stage-commit-push action (or 'auto').
      --no-refresh                    Use current machine snapshot without running a refresh scan first.
      --sync-strategy string          Sync strategy for sync-with-upstream and pre-push validation (rebase|merge). (default "rebase")
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.

