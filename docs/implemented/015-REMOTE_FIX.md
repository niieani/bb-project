right now when you run the `Clone locally` fix and it fails, I just see "one or more fixes failed" in the TUI. There's no indication of what failed.
I can run it manually, but I see a few issues:

```
bb fix markdown-llm clone
bb: fix: acquiring global lock
bb: loading config from /Users/bbrzoska/.config/bb-project/config.yaml
bb: using machine id "BazylisMBPMax"
bb: scan: snapshot is fresh (<= 1m0s), skipping refresh
bb: fix: collecting risk checks for 77 repositories
bb: fix: collecting risk checks (1/77)
bb: fix: collecting risk checks (10/77)
bb: fix: collecting risk checks (20/77)
bb: fix: collecting risk checks (30/77)
bb: fix: collecting risk checks (40/77)
bb: fix: collecting risk checks (50/77)
bb: fix: collecting risk checks (60/77)
bb: fix: collecting risk checks (70/77)
bb: fix: collecting risk checks (77/77)
bb: fix: verifying push access for 2 repositories with unknown access
bb: fix: verifying push access (1/2)
bb: fix: verifying push access (2/2)
bb: fix: push-access verification completed (1/2 updated)
bb: fix: released global lock
bb: fix: acquiring global lock for action clone
bb: loading config from ~/.config/bb-project/config.yaml
bb: using machine id "BazylisMBPMax"
bb: fix: released global lock for action clone
git clone https://github.com/niieani/markdown-llm.git /Volumes/Projects/Software/markdown-llm: exit status 128: Cloning into '/Volumes/Projects/Software/markdown-llm'...
fatal: could not read Username for 'https://github.com': terminal prompts disabled
```

issues:

- when I run a targetted `bb fix` it should not be running "collecting risk checks for 77 repositories" - it does not need information on any other repository than the one that's been selected to be fixed.
- "verifying push access for 2 repositories with unknown access" is only necessary for the repo in question
- "could not read Username for 'https://github.com': terminal prompts disabled" - when running from CLI, and not the TUI, we should always passthrough stdio for all synchronous commands.
- 2nd issue the above reveals - I think this comes from the fact that we created the project via `gh` CLI earlier - this defaulted to using the HTTPS protocol for git origin remote. We need to make user's preference configurable, moreover, they might want to custom-map their origin. For example I like to use a custom subdomain before the github domain, e.g. `git@niieani.github.com:niieani/bb-project.git` - this allows me to use the correct SSH key for the account (I have 2 github accounts, and another one is aliased `bbrzoska.github.com`). We need a way to update the remote URL based on the user's preference, which would be encoded in the global config file of `bb` (perhaps an optional config field such as "preferred GitHub remote URL" which would accept a template like `git@niieani.github.com:${org}/${repo}.git`. We should ensure that whenever we set the remote we use that template, and definitely when creating the project using the `gh` CLI, as well as offer a new `fix`, suggested for a new issue reason `remote_format_mismatch`.

Create a plan for addressing these issues comprehensively, and create a more concrete proposal.

**Proposed Plan (Concrete, TDD-first)**

1. **Fix TUI failure visibility first (quick UX win).**  
   In `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`, change immediate-apply completion handling so failed immediate actions transition to summary view (or at minimum show first error detail), instead of only setting `one or more fixes failed`.  
   In `/Volumes/Projects/Software/bb-project/internal/app/fix_tui_wizard.go`, reuse existing summary error rendering (`Error: ...`) for immediate failures.

2. **Split fix loading into two paths: interactive full-scan vs targeted single-repo.**  
   In `/Volumes/Projects/Software/bb-project/internal/app/fix.go`, keep `loadFixRepos` for interactive `bb fix` only. Add a targeted loader for `bb fix <project> [action]` that still refreshes snapshot but resolves selector first, then computes risk and push-access probe only for that target repo.  
   This removes `collecting risk checks for 77 repositories` and global unknown-push probing for targeted runs.

3. **Add CLI execution mode with stdio passthrough + prompt-enabled git for non-TUI runs.**  
   In `/Volumes/Projects/Software/bb-project/internal/gitx/git.go`, add runner execution options (attached stdio + prompt policy) instead of always forcing `GIT_TERMINAL_PROMPT=0`.  
   In `/Volumes/Projects/Software/bb-project/internal/app/fix.go` and `/Volumes/Projects/Software/bb-project/internal/app/clone.go`, use attached mode for non-TUI CLI flows, guarded by terminal detection, and keep non-interactive/prompt-disabled mode for TUI/background flows.

4. **Introduce GitHub remote URL template in config and centralize URL building.**  
   Add `github.remote_url_template` in `/Volumes/Projects/Software/bb-project/internal/domain/types.go` and validation/defaulting in `/Volumes/Projects/Software/bb-project/internal/state/store.go` and `/Volumes/Projects/Software/bb-project/internal/app/config.go`.  
   Recommended template semantics: support `${org}` (alias `${owner}`) and `${repo}`; when set, template overrides `github.remote_protocol`; when unset, current protocol behavior remains.  
   Add one helper (new file in `internal/app`, e.g. `github_remote_url.go`) and route all GitHub URL generation through it.

5. **Apply template everywhere remotes are set or derived.**  
   Update `/Volumes/Projects/Software/bb-project/internal/app/app.go`, `/Volumes/Projects/Software/bb-project/internal/app/clone.go`, `/Volumes/Projects/Software/bb-project/internal/app/fix.go`, and `/Volumes/Projects/Software/bb-project/internal/app/fix_action_spec.go` so init/create-project/fork/clone shorthand/http all use the same URL resolver.  
   Also make origin remote assignment idempotent (`add` or `set-url`) so `gh` behavior cannot leave wrong protocol/host.

6. **Add new fix reason + action for remote format mismatch.**  
   Add reason `remote_format_mismatch` and action (recommended id) `update-remote-url`.  
   Detect mismatch in fix-loading flow by comparing current GitHub remote URL with preferred URL from template/protocol.  
   Action behavior: if local repo exists, set effective remote URL; always update repo metadata origin URL to preferred value; revalidate afterwards.  
   Wire into eligibility, action spec, TUI reason/action mapping, summary guidance, and selection priority in `/Volumes/Projects/Software/bb-project/internal/app/fix.go`, `/Volumes/Projects/Software/bb-project/internal/app/fix_action_spec.go`, `/Volumes/Projects/Software/bb-project/internal/app/fix_tui.go`, and `/Volumes/Projects/Software/bb-project/internal/app/fix_tui_wizard.go`.

7. **TDD sequence (red → green).**  
   Add failing tests first for: immediate-apply failure detail surfacing; targeted fix does not log/run global risk/push-access passes; git runner prompt/stdio mode behavior; template validation/rendering; template usage in clone/init/create/fork; remote_format_mismatch eligibility and update action; docs/CLI behavior where applicable.  
   Then implement in small commits per phase.

8. **Docs and generated docs update.**  
   Update `/Volumes/Projects/Software/bb-project/README.md` (new config key, new fix action, mismatch reason behavior).  
   Regenerate command docs via `just docs-cli` (do not hand-edit `/Volumes/Projects/Software/bb-project/docs/cli/` or `/Volumes/Projects/Software/bb-project/docs/man/`).

---

**Concrete proposal decisions I recommend**

1. Treat `remote_format_mismatch` as a **fix issue reason** first (fix/TUI scope), not as a global sync blocker, to avoid unintended sync winner regressions.
2. For GitHub shorthand and GitHub HTTP repo links, always map to preferred template/protocol; for explicit raw git URLs, keep user input unchanged.
3. Update only the effective remote used by bb (preferred remote if set, otherwise origin/effective), and sync metadata `origin_url` to that value.
