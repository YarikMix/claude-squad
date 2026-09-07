package git

import (
	"strconv"
	"strings"
)

// DiffStats holds statistics about the changes in a diff
type DiffStats struct {
	// Content is the full diff content. Nothing computes this any more; it is retained
	// only for state.json compatibility (FromInstanceData may populate it from an older
	// saved state) and is always empty going forward.
	Content string
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

// IsEmpty reports whether the diff has no changes. The "&& d.Content == """ clause is
// always true going forward since nothing sets Content any more (see the field comment
// above); it stays for correctness against DiffStats restored from an older state.json.
func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// DiffNumstat returns the added/removed line counts between the worktree and the
// base branch without loading the full diff content into memory. This is the only
// way diff statistics are computed; it backs the +/- counters in the session list.
func (g *GitWorktree) DiffNumstat() *DiffStats {
	stats := &DiffStats{}

	// -N stages untracked files (intent to add), including them in the diff
	_, err := g.runGitCommand(g.worktreePath, "add", "-N", ".")
	if err != nil {
		stats.Error = err
		return stats
	}

	out, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", "--numstat", g.GetBaseCommitSHA())
	if err != nil {
		stats.Error = err
		return stats
	}

	stats.Added, stats.Removed = parseNumstat(out)
	return stats
}

// parseNumstat sums the added/removed columns from `git diff --numstat` output.
// Each line is formatted as <added>\t<removed>\t<path>. Binary files report
// "-\t-\t<path>" and are ignored for line totals.
func parseNumstat(out string) (added int, removed int) {
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 2 {
			continue
		}
		a, aerr := strconv.Atoi(fields[0])
		r, rerr := strconv.Atoi(fields[1])
		if aerr != nil || rerr != nil {
			continue
		}
		added += a
		removed += r
	}
	return added, removed
}
