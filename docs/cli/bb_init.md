## bb init

Initialize or adopt a repository and register metadata.

### Synopsis

Initialize or adopt a repository and register metadata.

Target resolution:
- With [project], bb creates or adopts at selected-catalog-root/project.
- The selected catalog is --catalog when provided, otherwise the machine default catalog.
- Without [project], bb infers the project from the current directory only when the current directory is inside the selected catalog and matches its repo layout depth.
- init does not run a post-init scan; run bb scan or bb sync when you want to refresh machine observations.

```
bb init [project] [flags]
```

### Options

```
      --catalog string   Select catalog instead of using the machine default.
  -h, --help             help for init
      --https            Use HTTPS remote protocol instead of SSH.
      --public           Create or register repository as public.
      --push             Allow initial push/upstream setup when local commits exist.
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.

