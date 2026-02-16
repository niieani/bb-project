## bb doctor

Report unsyncable repositories and reasons.

### Synopsis

Report unsyncable repositories and reasons.

When GitHub integration is configured (or selected repositories use GitHub remotes),
doctor also checks GitHub CLI prerequisites and emits warnings when gh is missing
or unauthenticated, including remediation guidance.

```
bb doctor [flags]
```

### Options

```
  -h, --help                          help for doctor
      --include-catalog stringArray   Limit scope to selected catalogs (repeatable).
```

### Options inherited from parent commands

```
  -q, --quiet   Suppress verbose bb logs.
```

### SEE ALSO

* [bb](bb.md)	 - Keep Git repositories consistent across machines.

