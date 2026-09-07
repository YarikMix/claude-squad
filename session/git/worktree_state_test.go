package git

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestWorktree builds a repository with one commit and a worktree on its own branch, and
// returns the GitWorktree pointing at it. The worktree is a real one, so the state checks run
// against real git output.
func newTestWorktree(t *testing.T) *GitWorktree {
	t.Helper()

	repoPath := filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, "", "init", repoPath)
	mustRunGit(t, repoPath, "config", "user.name", "Test User")
	mustRunGit(t, repoPath, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustRunGit(t, repoPath, "add", "README.md")
	mustRunGit(t, repoPath, "commit", "-m", "initial")

	worktreePath := filepath.Join(t.TempDir(), "worktree")
	mustRunGit(t, repoPath, "worktree", "add", "-b", "session-branch", worktreePath)

	return NewGitWorktreeFromStorage(repoPath, worktreePath, "session", "session-branch", "", false)
}

// addRemote publishes the repository to a bare clone and fetches it back, so the worktree's
// commits are reachable from a remote ref.
func addRemote(t *testing.T, g *GitWorktree) string {
	t.Helper()

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	mustRunGit(t, "", "init", "--bare", remotePath)
	mustRunGit(t, g.worktreePath, "remote", "add", "origin", remotePath)
	return remotePath
}

func TestDirtyFileCount_CleanWorktree(t *testing.T) {
	g := newTestWorktree(t)

	n, err := g.DirtyFileCount()
	if err != nil {
		t.Fatalf("DirtyFileCount: %v", err)
	}
	if n != 0 {
		t.Fatalf("clean worktree reported %d dirty files, want 0", n)
	}
}

func TestDirtyFileCount_CountsModifiedAndUntracked(t *testing.T) {
	g := newTestWorktree(t)

	if err := os.WriteFile(filepath.Join(g.worktreePath, "README.md"), []byte("changed\n"), 0644); err != nil {
		t.Fatalf("modify README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(g.worktreePath, "new.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	n, err := g.DirtyFileCount()
	if err != nil {
		t.Fatalf("DirtyFileCount: %v", err)
	}
	if n != 2 {
		t.Fatalf("got %d dirty files, want 2 (one modified, one untracked)", n)
	}
}

// A branch with no upstream is the common case here: sessions are created locally and only get
// one once the agent pushes. The count must work without one, since `git branch -D` deletes the
// branch either way.
func TestUnpushedCommitCount_NoRemoteAtAll(t *testing.T) {
	g := newTestWorktree(t)

	if err := os.WriteFile(filepath.Join(g.worktreePath, "work.txt"), []byte("work\n"), 0644); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	mustRunGit(t, g.worktreePath, "add", "work.txt")
	mustRunGit(t, g.worktreePath, "commit", "-m", "session work")

	n, err := g.UnpushedCommitCount()
	if err != nil {
		t.Fatalf("UnpushedCommitCount: %v", err)
	}
	if n != 2 {
		t.Fatalf("got %d unpushed commits, want 2 (initial plus session work)", n)
	}
}

func TestUnpushedCommitCount_PushedWorkCountsAsZero(t *testing.T) {
	g := newTestWorktree(t)
	addRemote(t, g)

	if err := os.WriteFile(filepath.Join(g.worktreePath, "work.txt"), []byte("work\n"), 0644); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	mustRunGit(t, g.worktreePath, "add", "work.txt")
	mustRunGit(t, g.worktreePath, "commit", "-m", "session work")
	mustRunGit(t, g.worktreePath, "push", "origin", "session-branch")

	n, err := g.UnpushedCommitCount()
	if err != nil {
		t.Fatalf("UnpushedCommitCount: %v", err)
	}
	if n != 0 {
		t.Fatalf("got %d unpushed commits after pushing, want 0", n)
	}
}

func TestUnpushedCommitCount_CountsCommitsMadeAfterThePush(t *testing.T) {
	g := newTestWorktree(t)
	addRemote(t, g)
	mustRunGit(t, g.worktreePath, "push", "origin", "session-branch")

	if err := os.WriteFile(filepath.Join(g.worktreePath, "later.txt"), []byte("later\n"), 0644); err != nil {
		t.Fatalf("write later file: %v", err)
	}
	mustRunGit(t, g.worktreePath, "add", "later.txt")
	mustRunGit(t, g.worktreePath, "commit", "-m", "work after the push")

	n, err := g.UnpushedCommitCount()
	if err != nil {
		t.Fatalf("UnpushedCommitCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("got %d unpushed commits, want 1", n)
	}
}

// git status --porcelain emits one line per file, but a path may contain spaces, so the count
// has to be over lines rather than fields.
func TestDirtyFileCount_HandlesPathsWithSpaces(t *testing.T) {
	g := newTestWorktree(t)

	if err := os.WriteFile(filepath.Join(g.worktreePath, "a file with spaces.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatalf("write spaced file: %v", err)
	}

	n, err := g.DirtyFileCount()
	if err != nil {
		t.Fatalf("DirtyFileCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("got %d dirty files, want 1", n)
	}
}
