# Resize wait guards widening but not narrowing

**Status:** open, accepted for now, documented in the e2e suite.
**Surfaced by:** the e2e harness (PR #16), `TestTerminalTabSurvivesResize`.

## Context

`TestTerminalTabSurvivesResize` resizes the outer tmux window and then asserts
the layout is intact (`requireLayoutIntact`). Between the resize and the
assertion the test must wait until `cs` has actually redrawn at the new size,
otherwise the assertion runs against the pre-resize screen and proves nothing.

The wait is `pane.WaitForWidth` in `e2e/harness_test.go`: it polls until the
widest captured row reaches the new width minus a tolerance. The tolerance is
8 columns, measured empirically — `capture-pane` trims lipgloss's center
padding, so the widest visible row lands a few columns short of the pane width.

## The gap

The wait distinguishes the new render from the old one only when the screen
gets **wider**:

- **80 -> 120 (widening):** a stale 80-column grid tops out at 80 columns, which
  cannot satisfy `>= 112`. The wait genuinely blocks until `cs` redraws. Guarded.
- **120 -> 80 (narrowing):** tmux immediately re-clips the still-un-redrawn
  120-column grid down to the 80-column pane, so rows are already at ~80 columns
  on the first poll. `>= 72` is satisfied by the stale frame. Effectively
  unguarded — the following `requireLayoutIntact(80, 30)` can run against the
  reflowed old screen.

`requireLayoutIntact`'s assertions (the four header/tab/list `Contains` checks)
are not width-sensitive, so the test does not currently give a false pass. But a
real `cs` regression specific to shrink-resize handling could slip past the
narrowing leg.

## Proposed improvement

Give the narrowing transition a positive signal that only the new render
produces. Options:

- Additionally require that **no** captured row exceeds the new width (a stale
  120-wide grid reflowed into 80 tends to leave wrapped remnants; a clean redraw
  does not), combined with the existing `>= width - tolerance` lower bound.
- Or watch a specific row whose length changes on redraw — e.g. the menu/tab bar
  that `app/app.go` sizes to the full window width — and wait for its length to
  match the new width.

Low priority: the current assertions are width-insensitive, so this closes a
latent gap rather than a live failure.

## References

- `e2e/harness_test.go` — `WaitForWidth`, `requireLayoutIntact`.
- `e2e/scenarios_test.go` — `TestTerminalTabSurvivesResize`.
- `app/app.go` — `updateHandleWindowSizeEvent` (the 0.3 / 0.9 split and the
  full-width menu row).
