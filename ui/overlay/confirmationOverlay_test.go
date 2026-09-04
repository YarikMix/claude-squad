package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// The dialog tells the user to press y or n, and those labels are printed on the keys
// themselves. Matching by physical position is what makes the instruction true in either
// layout: on a Russian keyboard the key labelled Y emits н, and the key labelled N emits т.
func TestConfirmationOverlayMatchesKeysByPosition(t *testing.T) {
	for _, tc := range []struct {
		key           string
		wantConfirmed bool
		why           string
	}{
		{"y", true, "the Latin key the dialog advertises"},
		{"n", false, "the Latin key the dialog advertises"},
		{"н", true, "sits where y sits, so it confirms"},
		{"т", false, "sits where n sits, so it cancels"},
	} {
		c := NewConfirmationOverlay("[!] Kill session 'x'?")
		confirmed, cancelled := false, false
		c.OnConfirm = func() { confirmed = true }
		c.OnCancel = func() { cancelled = true }

		require.True(t, c.HandleKeyPress(runeKey(tc.key)), "%q should close the dialog", tc.key)
		require.Equal(t, tc.wantConfirmed, confirmed, "%q: %s", tc.key, tc.why)
		require.Equal(t, !tc.wantConfirmed, cancelled, "%q: %s", tc.key, tc.why)
	}
}

// Escape cancels regardless of layout, and any other key leaves the dialog up, so a stray
// keystroke cannot answer a prompt that deletes a worktree and a branch.
func TestConfirmationOverlayEscapeAndUnknownKeys(t *testing.T) {
	c := NewConfirmationOverlay("[!] Kill session 'x'?")
	cancelled := false
	c.OnCancel = func() { cancelled = true }
	require.True(t, c.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}))
	require.True(t, cancelled)

	for _, key := range []string{"ф", "д", "z"} {
		other := NewConfirmationOverlay("[!] Kill session 'x'?")
		other.OnConfirm = func() { t.Fatalf("%q must not confirm", key) }
		other.OnCancel = func() { t.Fatalf("%q must not cancel", key) }
		require.False(t, other.HandleKeyPress(runeKey(key)),
			"%q is bound to neither answer and should leave the dialog open", key)
	}
}
