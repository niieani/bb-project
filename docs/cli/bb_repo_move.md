## bb repo move

Move a repository to a different catalog path.

```
bb repo move <repo> [flags]
```

### Options

```
      --as string        Target catalog-relative path override.
      --catalog string   Target catalog name.
      --dry-run          Preview move without mutating files or metadata.
  -h, --help             help for move
      --no-hooks         Skip configured post-move hooks.
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### SEE ALSO

* [bb repo](bb_repo.md)	 - Manage repository metadata and policy settings.

