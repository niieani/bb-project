# Work Plan

Implement metadata-aware clone resolution for registered projects.

## Scope

- Add red tests for cloning a registered repo by unique project name.
- Add red tests for cloning a registered repo by `owner/repo`.
- Update clone resolution so metadata can supply:
  - origin URL
  - catalog
  - canonical relative path
- Preserve existing behavior for unregistered remote inputs.
- Document the new behavior in the README.

## Validation

- Run focused app tests for clone behavior while iterating.
- Run a broader app package test pass before handoff.
