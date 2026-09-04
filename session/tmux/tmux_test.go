package tmux

import (
	cmd2 "claude-squad/cmd"
	"claude-squad/log"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"claude-squad/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

// TestMain initializes the logger the package's own code writes to. Without it a code path
// that logs panics on a nil logger during tests, which is a property of the test binary
// rather than of the code under test. The session and config packages do the same.
func TestMain(m *testing.M) {
	log.Initialize(false)
	defer log.Close()
	os.Exit(m.Run())
}

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

// Closing the attach channel has to be idempotent. Two paths reach it: the user pressing
// Ctrl+Q, and the session dying on its own while attached. Whichever runs second must not
// take the process down with a "close of closed channel" panic.
func TestCloseAttachChIsIdempotent(t *testing.T) {
	session := NewTmuxSessionWithDeps("twice", "claude", NewMockPtyFactory(t), cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
	})
	session.attachCh = make(chan struct{})

	require.NotPanics(t, func() {
		session.closeAttachCh()
		session.closeAttachCh()
	})

	require.Nil(t, session.attachCh, "the channel should be cleared once closed")
}

// The agent can exit from inside its pane (Ctrl+D), which takes the whole tmux session with
// it. Detaching afterwards used to panic on the failed re-attach, killing claude-squad and
// leaving the terminal in raw mode — and it panicked on exactly the key the on-screen error
// tells the user to press. A session that is simply gone is not a violated invariant.
func TestDetachDoesNotPanicWhenSessionIsGone(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	session := NewTmuxSessionWithDeps("gone", "claude", ptyFactory, cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			return nil
		},
	})

	// Stand in for the state Attach() leaves behind, without driving the real stdin loop.
	ptmx, err := ptyFactory.Start(exec.Command("true"))
	require.NoError(t, err)
	session.ptmx = ptmx
	session.attachCh = make(chan struct{})
	session.ctx, session.cancel = context.WithCancel(context.Background())
	session.wg = &sync.WaitGroup{}

	ch := session.attachCh
	require.NotPanics(t, session.Detach)

	select {
	case <-ch:
	default:
		t.Fatal("Detach must release the caller waiting on the attach channel")
	}
}

// The ordinary path must keep working: detaching from a session that is still alive
// re-attaches a fresh PTY and releases the caller. This is the case the cancel-before-close
// reordering in Detach could have broken.
func TestDetachReattachesWhenSessionIsAlive(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	session := NewTmuxSessionWithDeps("alive", "claude", ptyFactory, cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
	})

	ptmx, err := ptyFactory.Start(exec.Command("true"))
	require.NoError(t, err)
	session.ptmx = ptmx
	session.attachCh = make(chan struct{})
	session.ctx, session.cancel = context.WithCancel(context.Background())
	session.wg = &sync.WaitGroup{}

	ch := session.attachCh
	require.NotPanics(t, session.Detach)

	select {
	case <-ch:
	default:
		t.Fatal("Detach must release the caller waiting on the attach channel")
	}
	require.NotNil(t, session.ptmx, "a live session must be re-attached, leaving a valid ptmx")
	require.NotSame(t, ptmx, session.ptmx, "Restore should have opened a new PTY")
}
