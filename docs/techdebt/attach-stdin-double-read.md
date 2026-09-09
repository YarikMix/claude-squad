# Two readers of stdin during an attach

**Status:** open, worked around in the e2e tests, not fixed in `cs`.
**Surfaced by:** the e2e harness (PR #16), as a ~1-in-6 flake in the Linux CI job.

## Symptom

While a session is attached, a single keystroke is occasionally lost. Observed
on Linux (tmux 3.4) far more often than on macOS. In the e2e suite it showed up
as `exit 0` never executing, or `Ctrl+Q` never detaching, until the test's wait
timed out.

## Root cause

`cs` attaches from inside the Bubble Tea update loop
(`app/app.go`, the `keys.KeyEnter` handler calls `m.list.Attach()` / the
terminal tab's `AttachTerminal()` and blocks on the returned channel). Nothing
stops Bubble Tea's own input reader for the duration of the attach.

So during an attach two goroutines read `os.Stdin`:

- Bubble Tea's input reader (`bubbletea` `readLoop` → `readAnsiInputs`), and
- the attach forwarder in `session/tmux/tmux.go` (`Attach`), which copies stdin
  into the tmux pane.

The kernel hands each read to whichever goroutine wins. Bubble Tea's message
channel is unbuffered and its update loop is blocked inside the attach, so its
reader steals exactly one chunk of input and then parks on the channel send.
Usually that chunk is the terminal's reply to tmux's capability queries
(harmless). Sometimes it is the user's keypress — `Enter`, `Ctrl+Q` — which then
never reaches the pane.

A second, unrelated mechanism sits next to this one: the forwarder in
`session/tmux/tmux.go` discards all stdin for the first 50 ms after attach
(the "nuked first stdin" heuristic) to swallow those same capability replies.
It cannot tell a real keystroke typed within 50 ms from terminal garbage.

A real user rarely hits either: nobody types within 50 ms of pressing the attach
key. The e2e harness types instantly, which is why it exposed both.

## Current mitigation (in the tests, not in cs)

The harness resends the first post-attach keystroke until its effect appears —
what a person does when a key seems not to register. See
`e2e/harness_test.go` (`keysUntil`, `keysUntilGone`, `typeUntil`) and their use
in `e2e/scenarios_test.go`. Commit `340371b`.

This protects the tests only. Users are still exposed to the underlying loss.

## Proposed fix (its own PR)

Give the attach forwarder sole ownership of stdin for the life of the session,
and stop relying on the timing heuristic:

1. Release the terminal from Bubble Tea around the attach
   (`(*tea.Program).ReleaseTerminal` before `Attach`, `RestoreTerminal` after),
   so its input reader is not competing. Re-enter raw mode for the forwarder in
   between, since `ReleaseTerminal` restores cooked mode.
2. Rework the 50 ms "nuke" window in `session/tmux/tmux.go` so it discards only
   recognizable escape sequences (the DA / XTVERSION replies) rather than any
   input that arrives early. Otherwise step 1 alone just moves the loss from the
   double-reader into the nuke window (verified: a naive `ReleaseTerminal` fix
   made the e2e tests fail on exactly this).

Both touch the most fragile part of `cs` (attach/detach; see PRs #3 and #4), so
this deserves its own brainstorm, its own tests, and its own review — not a
ride-along in an unrelated change.

## Notes

- The double-reader is an upstream bug (`smtg-ai/claude-squad`), not specific to
  this fork; worth an upstream issue when picked up.
- When `cs` is fixed, the harness resends become harmless slack: they succeed on
  the first send when no input is lost.
