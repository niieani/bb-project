## bb scan

Discover repositories under catalogs and publish machine state.

```
bb scan [flags]
```

### Options

```
  -h, --help                          help for scan
      --include-catalog stringArray   Limit scope to selected catalogs (repeatable).
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### Notes

- Scan re-probes repositories whose cached `push_access` is `unknown` (including legacy unset values), even when the repository is not ahead.

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.
