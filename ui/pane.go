package ui

import "strings"

// fitHeight clamps rendered pane content to exactly height lines.
//
// Render first, then clamp — never the reverse. A pane style carries a width, and lipgloss
// wraps every line that exceeds it, so a block clamped before rendering grows back past the
// limit while it is being drawn. Nothing downstream catches that: lipgloss.Place, which seats
// the pane inside the tabbed window, returns content taller than its box exactly as it found
// it, and the surplus lines push the tab bar and the instance list off the top of the screen.
//
// keepTail decides which end survives a cut. The terminal follows a live session, so it keeps
// the newest lines; the preview keeps the oldest and passes an overflow marker, appended after
// the cut to show that content was dropped.
func fitHeight(rendered string, height int, keepTail bool, overflow string) string {
	if height <= 0 {
		return rendered
	}

	lines := strings.Split(rendered, "\n")
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

	return strings.Join(lines, "\n")
}
