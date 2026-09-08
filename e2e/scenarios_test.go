//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// createSession drives the list through `n`, the name, and Enter, and returns once the
// instance shows in the list and its agent has printed its banner into the preview.
func createSession(h *harness, title string) {
	h.t.Helper()
	h.Keys("n")
	h.Type(title)
	h.Keys("Enter")
	h.WaitFor(title)
	h.WaitFor("fake-agent ready")
}

func TestCreateSessionAndAttach(t *testing.T) {
	h := newHarness(t)
	createSession(h, "alpha")

	screen := h.Screen()
	require.Contains(t, screen, "fake-agent ready args=[]", "a first start must not carry restart_args")
	require.Len(t, h.Worktrees(), 1)
	require.True(t, h.BranchExists("e2e/alpha"))
	require.Equal(t, []string{tmuxPrefix + "alpha"}, h.inner.sessions())

	h.Keys("o")
	h.WaitNot("Instances")
	h.WaitFor("fake-agent ready args=[]")

	h.Keys("C-q")
	h.WaitFor("Instances")
	require.Equal(t, []string{tmuxPrefix + "alpha"}, h.inner.sessions(), "detaching keeps the session")
}
