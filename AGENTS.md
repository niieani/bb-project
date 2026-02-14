Build fully TDD: run tests as you keep implementing until you have completed the improvement.
Use latest features of Go wherever it makes sense and best practices + latest dependencies (feel free to check versions). We're on Go 1.26.0 right now.
Feel free to update the test harness if its missing features.
Default to parallelized tests for speed: add `t.Parallel()` to top-level tests and independent subtests, and keep fixtures/state isolated per test.

For interactive commands, use the charmbracelet ecosystem: [bubbletea](https://pkg.go.dev/github.com/charmbracelet/bubbletea) with [bubbles](https://github.com/charmbracelet/bubbles) component library and lipgloss for forms. Refer to full source code, READMEs and examples in:

- `references/vendor/bubbles`
- `references/vendor/bubbletea`
- `references/vendor/lipgloss`

When making changes, NEVER make them backwards compatibile, always clean-up the legacy code.
