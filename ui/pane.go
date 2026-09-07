package ui

import (
	"strings"

	"github.com/muesli/reflow/truncate"
)

// fitBox clamps rendered pane content to the pane's box: exactly height lines, none of them
// wider than width.
//
// Render first, then clamp — never the reverse. A pane style carries a width, and lipgloss
// wraps every line that exceeds it, so a block clamped before rendering grows back past the
// limit while it is being drawn. Nothing downstream catches either overflow: lipgloss.Place,
// which seats the pane inside the tabbed window, returns content larger than its box exactly as
// it found it. Surplus lines push the tab bar and the instance list off the top of the screen,
// and surplus columns widen the window past the tab bar, squeezing the instance list.
//
// The width clamp is needed because rendering does not guarantee the pane's own width. lipgloss
// wraps through ansi.Wrap, which does not count trailing whitespace against the limit; it
// normally trims such a run, but an escape sequence splitting the run defeats that. A shell
// prompt with a right-hand segment produces exactly that shape — powerlevel10k pads to the far
// edge and resets the colour — so a captured line comes back wider than the pane it was wrapped
// for, and lipgloss then pads the whole block out to match it.
//
// keepTail decides which end survives a height cut. The terminal follows a live session, so it
// keeps the newest lines; the preview keeps the oldest and passes an overflow marker, appended
// after the cut to show that content was dropped.
func fitBox(rendered string, width, height int, keepTail bool, overflow string) string {
	lines := strings.Split(rendered, "\n")

	if height > 0 {
		if len(lines) > height {
			if keepTail {
				lines = lines[len(lines)-height:]
			} else {
				lines = lines[:height]
			}
			if overflow != "" {
				lines = append(lines, overflow)
			}
		} else {
			lines = append(lines, make([]string, height-len(lines))...)
		}
	}

	if width > 0 {
		for i, line := range lines {
			lines[i] = truncate.String(line, uint(width))
		}
	}

	return strings.Join(lines, "\n")
}
