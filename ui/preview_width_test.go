package ui

import (
	"strings"
	"testing"

	"github.com/muesli/reflow/ansi"
	"github.com/stretchr/testify/require"
)

// hyperlink builds an OSC 8 sequence: an invisible URI, the visible label, then the closing
// sequence. Claude Code emits these for clickable links, so they turn up in captured panes.
func hyperlink(uri, label string) string {
	return "\x1b]8;id=1;" + uri + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}

const longURI = "http://qa:openopen@qaservices.example.com:42024/accounts/show?farm=cloud&type=proMini&status=free&limit=100000"

// The width helpers this package relies on understand CSI escapes only, so an OSC 8 URI is
// counted as visible text even though the terminal never draws it. That is the whole defect:
// a link labelled "click" measures 68 columns instead of 5.
func TestStripOSCSequencesMakesWidthHonest(t *testing.T) {
	link := hyperlink(longURI, "click")

	require.Greater(t, ansi.PrintableRuneWidth(link), 60,
		"precondition: the raw sequence is mis-measured, which is why this fix exists")
	require.Equal(t, 5, ansi.PrintableRuneWidth(stripOSCSequences(link)),
		"once the sequence is gone the width should be the label's")
}

// The label is content and must survive; the URI is metadata and must not.
func TestStripOSCSequencesKeepsTheLabel(t *testing.T) {
	out := stripOSCSequences("see " + hyperlink(longURI, "the report") + " for detail")

	require.Equal(t, "see the report for detail", out)
	require.NotContains(t, out, "qaservices.example.com")
}

// Only OSC is out-of-band. CSI colour codes are how the pane is coloured and must be left
// alone, or the preview turns monochrome.
func TestStripOSCSequencesKeepsColour(t *testing.T) {
	coloured := "\x1b[38;5;220mwarning\x1b[0m"

	require.Equal(t, coloured, stripOSCSequences(coloured))
}

// A BEL-terminated OSC is equally valid and appears in the wild.
func TestStripOSCSequencesHandlesBelTerminator(t *testing.T) {
	require.Equal(t, "title", stripOSCSequences("\x1b]0;window title\x07title"))
}

// The property that was violated: a pane must never render a line wider than itself. When it
// does, lipgloss pads the whole layout out to that width and PlaceOverlay centres the
// confirmation modal on it, pushing the modal off the right edge of the screen.
func TestPreviewPaneNeverRendersWiderThanThePane(t *testing.T) {
	const width, height = 60, 10

	p := NewPreviewPane()
	p.SetSize(width, height)
	p.previewState = previewState{text: strings.Repeat("run "+hyperlink(longURI, "task")+"\n", 3)}

	for i, line := range strings.Split(p.String(), "\n") {
		require.LessOrEqual(t, ansi.PrintableRuneWidth(line), width,
			"rendered preview line %d is wider than the pane", i)
	}
}

// The terminal tab renders captured content through its own path and needs the same bound.
func TestTerminalPaneNeverRendersWiderThanThePane(t *testing.T) {
	const width, height = 60, 12

	term := NewTerminalPane()
	term.SetSize(width, height)
	term.content = strings.Repeat("run "+hyperlink(longURI, "task")+"\n", 3)

	for i, line := range strings.Split(term.String(), "\n") {
		require.LessOrEqual(t, ansi.PrintableRuneWidth(line), width,
			"rendered terminal line %d is wider than the pane", i)
	}
}
