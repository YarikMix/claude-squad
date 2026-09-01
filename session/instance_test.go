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

// recordingPtyFactory behaves like nullPtyFactory but also records the tmux command used to
// start each PTY (e.g. "attach-session" vs "new-session"), and lets a test react to a
// command as it happens via onStart. Used by the Resume tests below to distinguish the
// Restore path from the create path, and to fake a session coming into existence once a
// new-session command has been issued.
type recordingPtyFactory struct {
	t       *testing.T
	ran     *[]string
	onStart func(cmdString string)
}

func (p *recordingPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	s := cmd.String()
	*p.ran = append(*p.ran, s)
	if p.onStart != nil {
		p.onStart(s)
	}
	return os.OpenFile(filepath.Join(p.t.TempDir(), "pty"), os.O_CREATE|os.O_RDWR, 0644)
}

func (p *recordingPtyFactory) Close() {}

// mustRunGit runs a git command against dir (or with no -C prefix when dir is empty) and
// fails the test on error. Mirrors the helper in session/git/worktree_ops_test.go.
func mustRunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmdArgs := args
	if dir != "" {
		cmdArgs = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, output)
	return string(output)
}

// newResumeTestRepo creates a real, minimal git repo at a fresh temp path (so
// IsBranchCheckedOut has something real to run against) and a worktree-shaped directory
// containing only a .git entry (so IsValidWorktree reports true and Resume skips Setup(),
// leaving only the tmux paths under test). It returns the repo path and the worktree path.
func newResumeTestRepo(t *testing.T) (repoPath, worktreePath string) {
	t.Helper()
	repoPath = filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, "", "init", repoPath)
	mustRunGit(t, repoPath, "config", "user.name", "Test User")
	mustRunGit(t, repoPath, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0644))
	mustRunGit(t, repoPath, "add", "README.md")
	mustRunGit(t, repoPath, "commit", "-m", "initial")

	worktreePath = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: fake\n"), 0644))
	return repoPath, worktreePath
}

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
// The error must point somewhere that actually works: Resume refuses anything but a Paused
// instance, so Restart has to park the instance as Paused itself before telling the user to
// press 'r' — otherwise the advice is a dead end.
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
	require.Equal(t, Paused, instance.Status,
		"the error tells the user to press 'r'; that only works once the instance is actually Paused")
}

// When the respawn itself fails (the session is still alive, just uncooperative), neither
// Running nor Paused would be an improvement, so Status must be left exactly as it was. This
// must hold at the same time as the assertion above: that one changes Status on the
// session-gone path, this one requires it unchanged on the respawn-failed path.
func TestRestartLeavesStatusUnchangedWhenRespawnFails(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "respawn-pane") {
				return fmt.Errorf("respawn-pane failed")
			}
			return nil // has-session (and everything else) succeeds: the session is alive
		},
	}

	instance, err := NewInstance(InstanceOptions{Title: "respawn-fails", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	instance.SetTmuxSession(tmux.NewTmuxSessionWithDeps("respawn-fails", "claude", &nullPtyFactory{t: t}, cmdExec))
	require.NoError(t, instance.Start(false))

	statusBefore := instance.Status
	err = instance.Restart()
	require.Error(t, err)
	require.Equal(t, statusBefore, instance.Status,
		"a failed respawn leaves the session alive; Status must not change")
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

// A tmux session that survived the pause is still running the program with its context, so
// Resume must Restore it, never restart it — restarting would throw away the very
// conversation this feature exists to preserve. This guards the plan's single most
// emphatic invariant: swapping this Restore() call for a start-with-restart-command call
// would otherwise compile and pass the rest of the suite silently.
func TestResumeRestoresSurvivingSessionWithoutRestartCommand(t *testing.T) {
	repoPath, worktreePath := newResumeTestRepo(t)

	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd.String())
			return nil // has-session succeeds: the session survived the pause
		},
	}
	ptyFactory := &recordingPtyFactory{t: t, ran: &ran}
	tmuxSession := tmux.NewTmuxSessionWithDeps("resume-survives", "claude", ptyFactory, cmdExec)
	// Configure the restart command as production code would have when this instance was
	// first created, so that "Resume does not restart" below is proven by the assertions,
	// not merely true because the restart command was never configured in the first place.
	tmuxSession.SetRestartCommand(restartCommandFor("claude"))

	instance, err := FromInstanceData(InstanceData{
		Title:   "resume-survives",
		Path:    repoPath,
		Branch:  "feature/test",
		Status:  Paused,
		Program: "claude",
		Worktree: GitWorktreeData{
			RepoPath:     repoPath,
			WorktreePath: worktreePath,
			SessionName:  "resume-survives",
			BranchName:   "feature/test",
		},
	})
	require.NoError(t, err)
	instance.SetTmuxSession(tmuxSession)

	require.NoError(t, instance.Resume())
	require.Equal(t, Running, instance.Status)

	require.NotEmpty(t, ran, "Resume should have reached the tmux paths, not returned early")
	joined := strings.Join(ran, "\n")
	require.Contains(t, joined, "attach-session", "a surviving session must be restored")
	require.NotContains(t, joined, "new-session", "a surviving session must not be recreated")
	require.NotContains(t, joined, "--continue",
		"restarting a surviving session would destroy the conversation it is preserving")
}

// A session that did not survive the pause has nothing left to preserve, so the path that
// recreates it must ask for the restart command, giving the agent back its conversation.
func TestResumeRecreatesGoneSessionWithRestartCommand(t *testing.T) {
	repoPath, worktreePath := newResumeTestRepo(t)

	var ran []string
	var sessionExists bool
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			s := cmd.String()
			ran = append(ran, s)
			if strings.Contains(s, "has-session") {
				if sessionExists {
					return nil
				}
				return fmt.Errorf("can't find session")
			}
			return nil
		},
	}
	ptyFactory := &recordingPtyFactory{
		t:   t,
		ran: &ran,
		onStart: func(s string) {
			if strings.Contains(s, "new-session") {
				// The session now "exists" for any has-session poll that follows.
				sessionExists = true
			}
		},
	}
	tmuxSession := tmux.NewTmuxSessionWithDeps("resume-gone", "claude", ptyFactory, cmdExec)
	tmuxSession.SetRestartCommand(restartCommandFor("claude"))

	instance, err := FromInstanceData(InstanceData{
		Title:   "resume-gone",
		Path:    repoPath,
		Branch:  "feature/test",
		Status:  Paused,
		Program: "claude",
		Worktree: GitWorktreeData{
			RepoPath:     repoPath,
			WorktreePath: worktreePath,
			SessionName:  "resume-gone",
			BranchName:   "feature/test",
		},
	})
	require.NoError(t, err)
	instance.SetTmuxSession(tmuxSession)

	require.NoError(t, instance.Resume())
	require.Equal(t, Running, instance.Status)

	require.NotEmpty(t, ran, "Resume should have reached the tmux paths, not returned early")
	var newSession string
	for _, c := range ran {
		if strings.Contains(c, "new-session") {
			newSession = c
		}
	}
	require.NotEmpty(t, newSession, "expected a new-session command, got: %v", ran)
	require.Contains(t, newSession, "claude --continue || claude")
}
