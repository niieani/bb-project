# Spec

## Motivation

`bb fix` summary after apply/revalidation hides its interactive affordances:

- follow-up fixes look like static text
- action row suggests two buttons, but only `enter`/`space` work
- the main summary body is not scrollable on short terminals

## Decisions

- Keep follow-up selection model: `↑/↓` moves active follow-up, `space` toggles
- Add explicit focus treatment to the active follow-up row so it reads as selectable
- Keep summary actions non-destructive by default:
  - no selected follow-ups: show `Skip` only
  - selected follow-ups: show `Skip` + `Run selected fixes`
- Make action buttons visually distinct by role
- Move summary body into a viewport with overflow indicators and short-window support
- Make the viewport follow the active follow-up row so selection stays visible while moving

## Validation

- red tests for summary action-row states
- red tests for active follow-up emphasis
- red tests for summary viewport overflow + cursor-follow scrolling
- targeted Go test run for fix TUI summary coverage
