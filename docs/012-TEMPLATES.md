# Spec/Plan: `bb init` Templates + Post-Create Hooks

## Summary

Add template-driven initialization to `bb init` with optional file scaffolding and hook execution before remote creation/push. Support named templates, raw repo/path template refs, default template selection, hook-only templates, and global/template post-create hooks. Add structured `bb config` wizard support for all new init-template/hook settings.

## Public Interfaces and Data Model Changes

### CLI (`/Volumes/Projects/Software/bb-project/internal/cli/cli.go`)

- Extend `bb init` flags:
- `--template <value>`: explicit template selector/name/repo/path.
- `--no-template`: disable template/default-template application for this run.
- Validation rule: `--template` and `--no-template` are mutually exclusive (exit code `2` with clear usage error).

### App Init Options (`/Volumes/Projects/Software/bb-project/internal/app/app.go`)

- Extend `app.InitOptions`:
- `Template string`
- `NoTemplate bool`

### Config Schema (`/Volumes/Projects/Software/bb-project/internal/domain/types.go`)

Add `init` block to `ConfigFile`:

```yaml
init:
  template_catalog: "" # existing machine catalog name used as template lookup root
  default_template: "" # template name or raw selector/repo/path
  post_create_hooks: [] # global hooks; each command supports shell or argv form
  templates: {} # named templates
```

Template shape:

```yaml
init:
  templates:
    node-api:
      source: "templates:node/api" # optional; if omitted => hook-only template
      post_create_hooks:
        - "npm install"
        - ["npx", "some-generator", ".", "--flag"]
```

Hook command type supports both forms:

- scalar string => shell command
- string array => argv command
- optional mapped form (for wizard persistence): `{ shell: "..." }` or `{ argv: ["..."] }`

### Defaults/Load/Save (`/Volumes/Projects/Software/bb-project/internal/state/store.go`)

- Seed defaults:
- `init.template_catalog = ""`
- `init.default_template = ""`
- `init.post_create_hooks = []`
- `init.templates = {}`
- Ensure load/save preserves empty maps/lists safely and deterministically.

### Validation (`/Volumes/Projects/Software/bb-project/internal/app/config.go`)

- Validate hook command shape (exactly one of shell/argv, non-empty).
- Validate template entries (non-empty names, valid hook commands).
- Keep `default_template` flexible (can be named template or raw selector).

## Runtime Behavior Spec (`bb init`)

### Template Selection

1. If `--no-template`, skip all template/default-template behavior.
2. Else if `--template` provided, use that value.
3. Else if `config.init.default_template` set, use that.
4. Else no template selected.

### Template Resolution (`--template` / default)

Resolution follows `bb link` selector spirit plus configured hook-only templates:

1. If selector matches `config.init.templates[selector]`, use that template config.
2. Resolve source from template `source` if set; if no `source`, this is a hook-only template.
3. If selector is not a named template, treat selector as ad-hoc source and resolve via:

- local selector resolution (same local selector logic as link),
- template catalog fallback using `init.template_catalog` and catalog path-depth rules,
- remote repo input parsing (`owner/repo`, GitHub URL, git URL) with temporary clone.

4. If named template source cannot be resolved:

- if template has hooks, continue as hook-only and emit warning.
- if template has no hooks, fail.

5. If ad-hoc source cannot be resolved, fail.

### Init Execution Ordering

For `RunInit`:

1. Resolve target path and ensure directory exists.
2. Ensure git repo exists (`git init -b main` when missing).
3. Read/validate origin:

- existing conflicting origin => fail (current behavior retained),
- existing matching origin => adopt path (creation flow disabled),
- missing origin => creation flow enabled.

4. Creation flow only (origin missing):

- apply template file copy (if source resolved),
- run template hooks, then global hooks,
- on any hook failure: abort before remote creation/push with remediation message (fix issue, remove folder, rerun).

5. Create remote and set origin.
6. Continue current metadata + optional push behavior.

### Template File Copy Rules

- Source content basis: source working tree.
- Exclusions: `.git` directory and gitignored paths.
- Copy strategy:
- non-conflicting files copy with mode/symlink preservation,
- conflict policy:
  - non-interactive terminal => fail with conflict summary,
  - interactive terminal => prompt per conflict with choices `overwrite`, `skip`, `abort`, plus `apply to all`.
- Copy never deletes destination files.

### Hook Execution Rules

- Hook order: template hooks first, global hooks second.
- Run directory: target repo directory.
- Shell form: run with `sh -lc`.
- Argv form: execute argv directly.
- Use attached stdio execution for subcommands (streaming behavior, no buffered suppression).
- Hook execution only in creation path (origin missing), never for idempotent adopt runs.

## `bb config` Wizard (Structured Support) (`/Volumes/Projects/Software/bb-project/internal/app/config_wizard.go`)

Add a new structured `Init` step with Bubble Tea/Bubbles/Lip Gloss patterns:

- `template_catalog` selector (existing machine catalogs + none).
- `default_template` input.
- global hooks table (add/edit/delete commands).
- templates table (add/edit/delete/set-default).
- template editor form:
- name,
- optional source,
- template hook commands table.
- hook command editor:
- mode toggle (`shell`/`argv`),
- value input,
- argv entered as JSON string array for unambiguous parsing.

Update wizard validation, diff/review rendering, step navigation labels/order, and focus/key handling per `/Volumes/Projects/Software/bb-project/docs/CLI-STYLE.md`.

## TDD Implementation Sequence

1. Add CLI red tests in `/Volumes/Projects/Software/bb-project/internal/cli/cli_test.go` for new flags and mutual exclusion.
2. Add config/model red tests in:

- `/Volumes/Projects/Software/bb-project/internal/state/store_test.go`
- `/Volumes/Projects/Software/bb-project/internal/app/config_test.go`
- new hook command marshal/unmarshal tests in `/Volumes/Projects/Software/bb-project/internal/domain/..._test.go`.

3. Add init behavior red tests in app layer:

- new file(s) under `/Volumes/Projects/Software/bb-project/internal/app/` for template resolution, source lookup, copy rules, hook ordering/failure, default/no-template behavior.

4. Add e2e red tests in `/Volumes/Projects/Software/bb-project/internal/e2e/init_test.go` for end-user flows.
5. Add wizard red tests in `/Volumes/Projects/Software/bb-project/internal/app/config_wizard_test.go` for new Init step and structured editors.
6. Implement minimal runtime to pass tests incrementally (types/load/validation -> CLI -> init engine -> conflict prompt -> wizard).
7. Update docs and regenerate CLI/man docs:

- `/Volumes/Projects/Software/bb-project/README.md`
- `/Volumes/Projects/Software/bb-project/docs/cli/*`
- `/Volumes/Projects/Software/bb-project/docs/man/man1/*`
- run `go run ./cmd/bb-docs`.

8. Run targeted suites continuously, then full `go test ./...`.

## Test Scenarios (Acceptance)

1. `bb init --template <named-template>` copies template files, runs template+global hooks, then creates remote.
2. `bb init` with `init.default_template` applies template automatically.
3. `bb init --no-template` bypasses default template and hooks tied to template.
4. Named hook-only template runs hooks even when source is absent.
5. Named template with missing source and no hooks fails clearly.
6. Ad-hoc unresolved template ref fails clearly.
7. Hook failure aborts init before remote creation/push and prints remediation guidance.
8. Template copy excludes `.git` and source gitignored files.
9. Conflict in non-interactive mode fails with conflict summary.
10. Conflict in interactive mode prompts and honors overwrite/skip/apply-to-all decisions.
11. Hooks do not run on adopt/idempotent init runs where origin already exists.
12. CLI docs/man and README reflect new flags/config behavior.

## Assumptions and Defaults Chosen

- Default template is opt-out via `--no-template`.
- `init.template_catalog` references existing machine catalog names, not raw paths.
- Working-tree template copy is authoritative; `.git` and gitignored files are excluded.
- Hook execution happens only in creation path (origin missing).
- Missing source for configured template is allowed only when hooks exist (hook-only fallback).
- Hook order is template hooks first, then global hooks.
