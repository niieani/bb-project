Build fully TDD: write red tests first, then implement. Run tests as you keep implementing until you have completed the improvement.
When I tell you to "implement" or "proceed" that's a shorthand for test-driven development (unless it doesn't make sense in the context - like if I'm refering to writing a document).
Update the test harness if its missing features required to complete your task.
If a refactor would make your task easier, and the codebase cleaner, propose it.
When making changes, NEVER make them backwards compatibile, always clean-up the legacy code.
Use latest features of Go wherever it makes sense; apply best practices, use latest dependencies (feel free to check versions). We're on Go 1.26.0 right now.
Default to parallelized tests for speed: add `t.Parallel()` to top-level tests and independent subtests, and keep fixtures/state isolated per test.

After finishing making code changes:

- update README.md and any related documentation
- start your response with a semantic git commit message with details about the change, include motivation and a short summary of the user's request
- never create branches or push automatically; only run branch creation/push when the user explicitly asks for it

If user gives you UI/UX feedback, update it with a generalized rule.

In non-interactive commands, whenever functionality demands running a subcommand (e.g. `git` or `gh` CLI), always passthrough stdio.

For interactive commands, use the charmbracelet ecosystem: [bubbletea](https://pkg.go.dev/github.com/charmbracelet/bubbletea) with [bubbles](https://github.com/charmbracelet/bubbles) component library and lipgloss for forms. Refer to full source code, READMEs and examples in:

- `references/vendor/bubbles`
- `references/vendor/bubbletea`
- `references/vendor/lipgloss`

Read ./docs/CLI-STYLE.md guide whenever touching any UI or interactive elements.
