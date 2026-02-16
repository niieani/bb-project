## bb clone

Clone repository into a catalog and register metadata/state.

```
bb clone <repo> [flags]
```

### Options

```
      --as string          Catalog-relative target path override.
      --catalog string     Select catalog to clone into.
      --filter string      Partial clone filter value (for example blob:none).
  -h, --help               help for clone
      --no-filter          Disable partial clone filter.
      --no-shallow         Disable shallow clone.
      --only stringArray   Sparse checkout path (repeatable).
      --shallow            Force shallow clone (depth=1).
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.

