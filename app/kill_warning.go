package app

import "fmt"

// killWarning builds the confirmation text for killing a session, naming the work that will not
// survive it.
//
// Killing is the only way to remove a session, and it is not gentle: the worktree goes with
// `git worktree remove -f`, whose -f overrides git's refusal to discard uncommitted changes, and
// the branch goes with `git branch -D`, which discards commits no remote has. Neither loss is
// recoverable, and the plain prompt gave no hint that either was about to happen.
func killWarning(title string, dirtyFiles, unpushedCommits int) string {
	header := fmt.Sprintf("[!] Kill session '%s'?", title)

	files := "files"
	if dirtyFiles == 1 {
		files = "file"
	}
	commits, they, are := "commits", "They", "are"
	if unpushedCommits == 1 {
		commits, they, are = "commit", "It", "is"
	}

	switch {
	case dirtyFiles > 0 && unpushedCommits > 0:
		return fmt.Sprintf("%s\n\nUncommitted changes in %d %s.\n%d %s %s on no remote.\n\nBoth will be lost.",
			header, dirtyFiles, files, unpushedCommits, commits, are)
	case dirtyFiles > 0:
		return fmt.Sprintf("%s\n\nUncommitted changes in %d %s will be lost.", header, dirtyFiles, files)
	case unpushedCommits > 0:
		return fmt.Sprintf("%s\n\n%d %s %s on no remote. %s will be lost.",
			header, unpushedCommits, commits, are, they)
	default:
		return header
	}
}
