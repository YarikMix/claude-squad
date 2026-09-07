package app

import "testing"

func TestKillWarning(t *testing.T) {
	tests := []struct {
		name     string
		dirty    int
		unpushed int
		want     string
	}{
		{
			name: "nothing to lose",
			want: "[!] Kill session 'duty'?",
		},
		{
			name:  "uncommitted changes only",
			dirty: 3,
			want:  "[!] Kill session 'duty'?\n\nUncommitted changes in 3 files will be lost.",
		},
		{
			name:  "a single uncommitted file reads as one file",
			dirty: 1,
			want:  "[!] Kill session 'duty'?\n\nUncommitted changes in 1 file will be lost.",
		},
		{
			name:     "unpushed commits only",
			unpushed: 2,
			want:     "[!] Kill session 'duty'?\n\n2 commits are on no remote. They will be lost.",
		},
		{
			name:     "a single unpushed commit reads as one commit",
			unpushed: 1,
			want:     "[!] Kill session 'duty'?\n\n1 commit is on no remote. It will be lost.",
		},
		{
			name:     "both, each a single item",
			dirty:    1,
			unpushed: 1,
			want: "[!] Kill session 'duty'?\n\nUncommitted changes in 1 file.\n" +
				"1 commit is on no remote.\n\nBoth will be lost.",
		},
		{
			name:     "both",
			dirty:    3,
			unpushed: 2,
			want: "[!] Kill session 'duty'?\n\nUncommitted changes in 3 files.\n" +
				"2 commits are on no remote.\n\nBoth will be lost.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := killWarning("duty", tt.dirty, tt.unpushed)
			if got != tt.want {
				t.Errorf("killWarning(%d, %d)\n got: %q\nwant: %q", tt.dirty, tt.unpushed, got, tt.want)
			}
		})
	}
}
