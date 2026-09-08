//go:build e2e

package e2e

import (
	"path/filepath"
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

func TestKillWarnsAboutUnsavedWork(t *testing.T) {
	h := newHarness(t)
	createSession(h, "alpha")
	wt := h.SoleWorktree()

	// One commit nobody has, one tracked file modified and not committed.
	h.CommitInWorktree(wt, "done.txt", "committed\n")
	writeFile(t, filepath.Join(wt, "README.md"), "uncommitted change\n")

	h.Keys("D")
	screen := h.WaitFor("Kill session 'alpha'?")
	require.Contains(t, screen, "Uncommitted changes in 1 file")
	require.Contains(t, screen, "1 commit is on no remote")
	require.Contains(t, screen, "Both will be lost")

	h.Keys("n")
	h.WaitNot("Kill session")
	require.Contains(t, h.Screen(), "alpha")
	require.Len(t, h.Worktrees(), 1, "cancelling must not touch the worktree")
	require.True(t, h.BranchExists("e2e/alpha"))
}

func TestKillRemovesACleanSessionWithoutWarning(t *testing.T) {
	h := newHarness(t)
	createSession(h, "alpha")
	wt := h.SoleWorktree()
	h.CommitInWorktree(wt, "done.txt", "committed\n")
	h.PushBranch(wt)

	h.Keys("D")
	screen := h.WaitFor("Kill session 'alpha'?")
	require.NotContains(t, screen, "will be lost")
	require.NotContains(t, screen, "Uncommitted")
	require.NotContains(t, screen, "no remote")

	h.Keys("y")
	h.WaitFor("No agents running yet")
	require.Empty(t, h.Worktrees())
	require.False(t, h.BranchExists("e2e/alpha"))
	require.Empty(t, h.inner.sessions())
}

func TestResumeAfterTmuxServerDies(t *testing.T) {
	h := newHarness(t)
	createSession(h, "alpha")
	wtBefore := h.SoleWorktree()

	h.Quit()
	h.KillInnerServer()
	require.Empty(t, h.inner.sessions())
	h.Relaunch()

	screen := h.WaitFor("Session is paused. Press 'r' to resume.")
	require.Contains(t, screen, "alpha")
	require.Contains(t, screen, "resume", "the menu must offer r")

	h.Keys("r")
	h.WaitFor("fake-agent ready args=[--continue]")
	require.Equal(t, wtBefore, h.SoleWorktree(), "resume reuses the worktree on disk")
	require.True(t, h.BranchExists("e2e/alpha"))
	require.Equal(t, []string{tmuxPrefix + "alpha"}, h.inner.sessions())
}

func TestRestartKeepsTheSessionAndAppendsRestartArgs(t *testing.T) {
	h := newHarness(t)
	createSession(h, "alpha")
	agent := h.Instance("alpha")

	agent.Type("echo marker")
	agent.Keys("Enter")
	h.WaitFor("fake-agent: marker")

	h.Keys("R")
	h.WaitFor("fake-agent ready args=[--continue]")
	h.WaitNot("fake-agent: marker")
	require.Equal(t, []string{tmuxPrefix + "alpha"}, h.inner.sessions(), "restart respawns the pane, not the session")
	require.Equal(t, "alpha", h.WindowName("alpha"))

	// Second act: the same restart from inside the attached session.
	agent.Type("echo second")
	agent.Keys("Enter")
	h.WaitFor("fake-agent: second")
	h.Keys("o")
	h.WaitNot("Instances")
	h.WaitFor("fake-agent: second")

	h.Keys("C-x")
	h.WaitNot("fake-agent: second")
	screen := h.WaitFor("fake-agent ready args=[--continue]")
	require.NotContains(t, screen, "Instances", "Ctrl+X keeps the user attached")
	require.Equal(t, "alpha", h.WindowName("alpha"))

	h.Keys("C-q")
	h.WaitFor("Instances")
}

func TestAgentExitInsideThePaneReturnsToTheList(t *testing.T) {
	h := newHarness(t)
	createSession(h, "alpha")
	wt := h.SoleWorktree()

	h.Keys("o")
	h.WaitNot("Instances")

	// Typed into the attached pane, so it reaches the agent through cs's stdin forwarder.
	h.Type("exit 0")
	h.Keys("Enter")

	screen := h.WaitFor("Instances")
	require.Contains(t, screen, "Session is paused. Press 'r' to resume.")
	require.Contains(t, screen, "r resume", "the menu must offer r")
	require.NotContains(t, screen, "Session terminated without detaching")
	require.Empty(t, h.inner.sessions())

	// After the session ends underneath an attach, cs's stdin forwarder is still blocked in
	// a read and swallows the next keystroke (documented in PR #4). Spend a harmless key on
	// it: Down does nothing in a one-entry list whether or not it is consumed.
	h.Keys("Down")
	h.Keys("r")
	h.WaitFor("fake-agent ready args=[--continue]")
	require.Equal(t, wt, h.SoleWorktree())
}
