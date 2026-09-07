package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// wideLines builds count lines, each far wider than width, tagged so a test can tell which
// of them survived a cut. A pane style carries a width, and lipgloss wraps anything past it,
// so every one of these lines becomes several once rendered.
func wideLines(count, width int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		b.WriteString(fmt.Sprintf("line-%02d-", i))
		b.WriteString(strings.Repeat("x", width*2))
		b.WriteString("\n")
	}
	return b.String()
}

// The defect this guards: content was clamped to the pane height before being rendered, and
// rendering then wrapped the long lines back past that limit. lipgloss.Place, which seats the
// pane inside the tabbed window, leaves content taller than its box alone, so the surplus
// pushed the tab bar and the instance list off the top of the screen.
func TestTerminalPaneNeverRendersTallerThanThePane(t *testing.T) {
	const width, height = 60, 12

	term := NewTerminalPane()
	term.SetSize(width, height)
	term.content = wideLines(20, width)

	require.LessOrEqual(t, len(strings.Split(term.String(), "\n")), height,
		"rendered terminal is taller than the pane")
}

// One overlong line among short ones is enough to overflow the pane, which is why the real
// case only showed up on sessions whose prompt happened to reach the edge.
func TestTerminalPaneStaysBoundedWithASingleWideLine(t *testing.T) {
	const width, height = 60, 12

	term := NewTerminalPane()
	term.SetSize(width, height)
	term.content = strings.Repeat("short\n", height-1) + strings.Repeat("x", width*3)

	require.LessOrEqual(t, len(strings.Split(term.String(), "\n")), height,
		"rendered terminal is taller than the pane")
}

// The terminal follows a live session, so a cut has to drop the oldest lines, not the newest.
func TestTerminalPaneKeepsTheNewestLines(t *testing.T) {
	const width, height = 60, 12

	term := NewTerminalPane()
	term.SetSize(width, height)
	term.content = wideLines(20, width)

	rendered := term.String()
	require.Contains(t, rendered, "line-19-", "newest line was dropped")
	require.NotContains(t, rendered, "line-00-", "oldest line survived a cut")
}

// Content shorter than the pane is padded out, so the pane always occupies its full box.
func TestTerminalPaneFillsThePaneWhenContentIsShort(t *testing.T) {
	const width, height = 60, 12

	term := NewTerminalPane()
	term.SetSize(width, height)
	term.content = "one\ntwo\n"

	require.Equal(t, height, len(strings.Split(term.String(), "\n")),
		"short content should be padded to the pane height")
}

// The preview pane clamps through the same path and needs the same bound.
func TestPreviewPaneNeverRendersTallerThanThePane(t *testing.T) {
	const width, height = 60, 12

	p := NewPreviewPane()
	p.SetSize(width, height)
	p.previewState = previewState{text: wideLines(20, width)}

	require.LessOrEqual(t, len(strings.Split(p.String(), "\n")), height,
		"rendered preview is taller than the pane")
}

// The preview keeps the top of the content and marks the cut, unlike the terminal.
func TestPreviewPaneMarksTruncatedContent(t *testing.T) {
	const width, height = 60, 12

	p := NewPreviewPane()
	p.SetSize(width, height)
	p.previewState = previewState{text: wideLines(20, width)}

	lines := strings.Split(p.String(), "\n")
	require.Contains(t, strings.Join(lines, "\n"), "line-00-", "oldest line was dropped")
	require.Equal(t, "...", strings.TrimSpace(lines[len(lines)-1]),
		"a cut should be marked with an ellipsis on the last line")
}

// The pane bounds matter because the tabbed window has no bound of its own: lipgloss.Place
// returns over-tall content untouched, so a pane that overflows carries the whole window past
// the screen height. The terminal then scrolls, and the tab bar and instance list disappear off
// the top — the symptom users see. Before the panes clamped after rendering, an 80x30 window
// holding wrapped content rendered 99 lines.
func TestTabbedWindowNeverRendersTallerThanTheScreen(t *testing.T) {
	const width, height = 80, 30

	term := NewTerminalPane()
	w := NewTabbedWindow(NewPreviewPane(), term)
	w.SetSize(width, height)
	w.Toggle() // Preview -> Terminal
	term.content = wideLines(40, width)

	require.LessOrEqual(t, len(strings.Split(w.String(), "\n")), height,
		"tabbed window is taller than the screen, which scrolls the tab bar out of view")
}
