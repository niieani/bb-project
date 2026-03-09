# CLI Rules

## Post-action summaries with follow-up fixes

- If a summary screen offers automated follow-up fixes, render the active follow-up with explicit focus styling; a bare cursor glyph is not enough.
- Do not show a primary action button until its prerequisite selection exists. When no follow-up is selected, show only the escape-hatch action (`Skip`/back).
- Summary/detail screens must remain usable on short terminals: scroll the body in a viewport, keep the active selection visible, and surface overflow with clear top/bottom indicators.
