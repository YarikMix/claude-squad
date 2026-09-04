package ui

import "regexp"

// oscSequence matches an OSC control string: ESC ] followed by the payload and terminated by
// either BEL or ST (ESC \). Neither the payload nor the terminator may contain another ESC or
// BEL, which is what bounds the match.
var oscSequence = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

// stripOSCSequences removes OSC control strings from captured pane content.
//
// Terminals use OSC for out-of-band data, most visibly OSC 8 hyperlinks, whose URI is
// metadata the terminal never draws. The width helpers this package renders through
// understand CSI escapes only, so they count that URI as visible text: a link labelled
// "click" measures 68 columns instead of 5. Every consumer downstream inherits the inflated
// figure — lipgloss wraps lines that already fit, and PlaceOverlay centres the confirmation
// modal on the widest line it can find, which pushes the modal off the right edge of the
// screen where the user cannot read which session they are about to kill.
//
// Dropping the sequences leaves the visible label and makes the measurements honest. The
// links stop being clickable in the preview; attaching to the session still shows the real
// pane, where they work as before.
func stripOSCSequences(s string) string {
	return oscSequence.ReplaceAllString(s, "")
}
