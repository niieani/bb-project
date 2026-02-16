## bb repo access-refresh

Probe and refresh cached repository push access.

```
bb repo access-refresh <repo> [flags]
```

### Options

```
  -h, --help   help for access-refresh
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### Notes

- For GitHub origins (including `*.github.com` aliases), access refresh consults `gh` viewer permission and then falls back to `git push --dry-run` when needed.
- For non-GitHub origins, access refresh uses `git push --dry-run` (or leaves access `unknown` when probing is unsupported).

### SEE ALSO

* [bb repo](bb_repo.md)	 - Manage repository metadata and policy settings.
