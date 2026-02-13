## bb sync

Run observe, publish, and reconcile flow.

```
bb sync [flags]
```

### Options

```
      --dry-run                       Show reconcile decisions without write-side sync actions.
  -h, --help                          help for sync
      --include-catalog stringArray   Limit scope to selected catalogs (repeatable).
      --notify                        Emit notifications for unsyncable repositories.
      --push                          Allow pushing ahead commits when policy blocks by default.
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.

