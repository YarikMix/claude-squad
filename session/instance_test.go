package session

import (
	"claude-squad/cmd/cmd_test"
	"claude-squad/log"
	"claude-squad/session/tmux"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.Initialize(false)
	defer log.Close()
	// Pin the restart args so the suite neither reads nor creates the developer's real
	// config file. Individual tests override this where the value matters.
	loadRestartArgs = func() string { return "--continue" }
	os.Exit(m.Run())
}

// nullPtyFactory hands back a throwaway file instead of a real PTY.
type nullPtyFactory struct {
	t     *testing.T
	calls int
}

func (p *nullPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	p.calls++
	return os.OpenFile(filepath.Join(p.t.TempDir(), "pty"), os.O_CREATE|os.O_RDWR, 0644)
}

func (p *nullPtyFactory) Close() {}

// When the tmux server dies between runs, every session goes with it while the worktree
// and branch survive on disk. Restoring such an instance must park it as Paused so the
// user can resume it. Returning an error instead is not an option: LoadInstances aborts on
// the first failure, so one dead session would hide every other instance.
// See https://github.com/smtg-ai/claude-squad/issues/216.
func TestStartPausesInstanceWhenTmuxSessionNoLongerExists(t *testing.T) {
	ptyFactory := &nullPtyFactory{t: t}
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			return nil
		},
	}

	instance, err := NewInstance(InstanceOptions{Title: "revived", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("revived", "claude", ptyFactory, cmdExec))

	require.NoError(t, instance.Start(false), "a dead tmux session is recoverable, not a startup failure")
	require.Equal(t, Paused, instance.Status)
	require.True(t, instance.Started())
	require.Zero(t, ptyFactory.calls, "should not attach to a session that does not exist")
}

// The happy path is unchanged: an instance whose session survived comes back Running.
func TestStartRestoresInstanceWhenTmuxSessionSurvives(t *testing.T) {
	ptyFactory := &nullPtyFactory{t: t}
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
	}

	instance, err := NewInstance(InstanceOptions{Title: "alive", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("alive", "claude", ptyFactory, cmdExec))

	require.NoError(t, instance.Start(false))
	require.Equal(t, Running, instance.Status)
	require.Equal(t, 1, ptyFactory.calls)
}

// Restarting respawns the pane's process with the restart args appended, so the agent comes
// back holding the conversation. The tmux session, the worktree and the branch are untouched.
func TestRestartRespawnsPaneWithRestartArgs(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd.String())
			return nil
		},
	}

	instance, err := NewInstance(InstanceOptions{Title: "restarted", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("restarted", "claude", &nullPtyFactory{t: t}, cmdExec))
	require.NoError(t, instance.Start(false))

	require.NoError(t, instance.Restart())
	require.Equal(t, Running, instance.Status)

	var respawn string
	for _, c := range ran {
		if strings.Contains(c, "respawn-pane") {
			respawn = c
		}
	}
	require.NotEmpty(t, respawn, "expected a respawn-pane command, got: %v", ran)
	require.Contains(t, respawn, "-k -t claudesquad_restarted claude --continue || claude")
	require.NotContains(t, strings.Join(ran, "\n"), "kill-session",
		"restart must not tear the session down")
}

// A paused instance has no pane to respawn: its worktree is gone and the process is dead.
// Resume is the operation that rebuilds it, so say so instead of failing obscurely.
func TestRestartOnPausedInstanceReturnsError(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
	}

	instance, err := NewInstance(InstanceOptions{Title: "paused", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("paused", "claude", &nullPtyFactory{t: t}, cmdExec))
	require.NoError(t, instance.Start(false))
	instance.SetStatus(Paused)

	err = instance.Restart()
	require.Error(t, err)
	require.Contains(t, err.Error(), "'r'", "the error should point the user at resume")
}

// The tmux server can die between runs, leaving a Running instance whose session is gone.
func TestRestartWhenTmuxSessionIsGoneReturnsError(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			return nil
		},
	}

	instance, err := NewInstance(InstanceOptions{Title: "gone", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("gone", "claude", &nullPtyFactory{t: t}, cmdExec))
	require.NoError(t, instance.Start(false))
	// Start parks an instance with a dead session as Paused; drop that so we exercise the
	// session-existence check rather than the paused one.
	instance.SetStatus(Running)

	err = instance.Restart()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no longer exists")
}

// An instance that has never been started has nothing to restart.
func TestRestartOnUnstartedInstanceReturnsError(t *testing.T) {
	instance, err := NewInstance(InstanceOptions{Title: "never", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)

	require.Error(t, instance.Restart())
}

// Programs with no resume flag must still restart, just without extra args.
func TestRestartWithEmptyRestartArgsRestartsBareProgram(t *testing.T) {
	previous := loadRestartArgs
	loadRestartArgs = func() string { return "" }
	t.Cleanup(func() { loadRestartArgs = previous })

	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd.String())
			return nil
		},
	}

	instance, err := NewInstance(InstanceOptions{Title: "bare", Path: t.TempDir(), Program: "aider"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("bare", "aider", &nullPtyFactory{t: t}, cmdExec))
	require.NoError(t, instance.Start(false))

	require.NoError(t, instance.Restart())
	require.Contains(t, strings.Join(ran, "\n"), "respawn-pane -k -t claudesquad_bare aider")
	require.NotContains(t, strings.Join(ran, "\n"), "||")
}
