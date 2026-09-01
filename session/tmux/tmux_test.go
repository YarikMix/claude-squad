package tmux

import (
	cmd2 "claude-squad/cmd"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"claude-squad/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

type MockPtyFactory struct {
	t *testing.T

	// Array of commands and the corresponding file handles representing PTYs.
	cmds  []*exec.Cmd
	files []*os.File
}

func (pt *MockPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	filePath := filepath.Join(pt.t.TempDir(), fmt.Sprintf("pty-%s-%d", pt.t.Name(), rand.Int31()))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		pt.cmds = append(pt.cmds, cmd)
		pt.files = append(pt.files, f)
	}
	return f, err
}

func (pt *MockPtyFactory) Close() {}

func NewMockPtyFactory(t *testing.T) *MockPtyFactory {
	return &MockPtyFactory{
		t: t,
	}
}

func TestSanitizeName(t *testing.T) {
	session := NewTmuxSession("asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf", session.sanitizedName)

	session = NewTmuxSession("a sd f . . asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf__asdf", session.sanitizedName)
}

func TestStartTmuxSession(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s claudesquad_test-session -c %s claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
	require.Equal(t, "tmux attach-session -t claudesquad_test-session",
		cmd2.ToString(ptyFactory.cmds[1]))

	require.Equal(t, 2, len(ptyFactory.files))

	// File should be closed.
	_, err = ptyFactory.files[0].Stat()
	require.Error(t, err)
	// File should be open
	_, err = ptyFactory.files[1].Stat()
	require.NoError(t, err)
}

// A tmux server that has gone away (reboot, crash, `tmux kill-server`) takes every session
// with it. attach-session against a missing session still forks successfully, so Restore
// has to check for the session itself or it reports success while attached to nothing.
func TestRestoreReturnsErrSessionNotFoundWhenSessionIsGone(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("gone", "program", ptyFactory, cmdExec)
	err := session.Restore()

	require.ErrorIs(t, err, ErrSessionNotFound)
	require.Empty(t, ptyFactory.cmds, "should not have opened a PTY for a session that does not exist")
}

func TestRestoreAttachesWhenSessionExists(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
	}

	session := NewTmuxSessionWithDeps("alive", "program", ptyFactory, cmdExec)
	require.NoError(t, session.Restore())
	require.Len(t, ptyFactory.cmds, 1)
	require.Contains(t, ptyFactory.cmds[0].String(), "attach-session")
}

func TestBuildRestartCommand(t *testing.T) {
	// tmux hands a single shell-command argument to the shell, so the `||` fallback is
	// honored. Without it, `claude --continue` in a worktree with no conversation on disk
	// exits non-zero and takes the pane — and the whole tmux session — down with it.
	require.Equal(t, "claude --continue || claude", BuildRestartCommand("claude", "--continue"))

	// Flags already in the program must survive the restart.
	require.Equal(t,
		"claude --add-dir ~/projects/foo --continue || claude --add-dir ~/projects/foo",
		BuildRestartCommand("claude --add-dir ~/projects/foo", "--continue"))

	// Programs with no resume flag restart as-is.
	require.Equal(t, "aider", BuildRestartCommand("aider", ""))
	require.Equal(t, "aider", BuildRestartCommand("aider", "   "))
	require.Equal(t, "", BuildRestartCommand("", "--continue"))
}

func TestRespawnPaneUsesRestartCommand(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd2.ToString(cmd))
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("restarted", "claude", NewMockPtyFactory(t), cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.NoError(t, session.RespawnPane())
	require.Contains(t, ran,
		"tmux respawn-pane -k -t claudesquad_restarted claude --continue || claude")
}

// With no restart command configured, a restart still has to bring the program back up.
func TestRespawnPaneFallsBackToProgram(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd2.ToString(cmd))
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("plain", "aider", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, session.RespawnPane())
	require.Contains(t, ran, "tmux respawn-pane -k -t claudesquad_plain aider")
}

// Respawning a pane of a session that no longer exists cannot work; callers need to tell
// that apart from a failed respawn so they can point the user at resume instead.
func TestRespawnPaneReturnsErrSessionNotFoundWhenSessionIsGone(t *testing.T) {
	respawned := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			if strings.Contains(cmd.String(), "respawn-pane") {
				respawned = true
			}
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("gone", "claude", NewMockPtyFactory(t), cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.ErrorIs(t, session.RespawnPane(), ErrSessionNotFound)
	require.False(t, respawned, "should not respawn a pane of a session that does not exist")
}

// Resume rebuilds a session whose tmux server died. That new session is where the
// conversation is lost today, so it starts the restart command rather than the bare program.
func TestStartWithRestartCommandStartsRestartCommand(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("output"), nil },
	}

	workdir := t.TempDir()
	session := newTmuxSession("resumed", "claude", ptyFactory, cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.NoError(t, session.StartWithRestartCommand(workdir))
	require.Equal(t, fmt.Sprintf(
		"tmux new-session -d -s claudesquad_resumed -c %s claude --continue || claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}

// A brand new session must not get the restart args: a fresh worktree has no conversation,
// and `claude --continue` there would fail on the very first start.
func TestStartIgnoresRestartCommand(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("output"), nil },
	}

	workdir := t.TempDir()
	session := newTmuxSession("fresh", "claude", ptyFactory, cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.NoError(t, session.Start(workdir))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s claudesquad_fresh -c %s claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}
