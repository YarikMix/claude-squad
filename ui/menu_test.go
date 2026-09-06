package ui

import (
	"claude-squad/session"
	"regexp"
	"strings"
	"testing"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// renderLine returns the menu's rendered text with ANSI styling stripped and the
// lipgloss.Place centering padding trimmed, so tests can compare against plain
// expected strings.
func renderLine(m *Menu) string {
	return strings.TrimSpace(ansiEscape.ReplaceAllString(m.String(), ""))
}

func newTestInstance(t *testing.T, status session.Status) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: "x", Path: t.TempDir(), Program: "claude"})
	if err != nil {
		t.Fatalf("session.NewInstance: %v", err)
	}
	inst.SetStatus(status)
	return inst
}

func TestMenuString(t *testing.T) {
	const (
		running = "n new • D kill │ ↵/o open • R restart │ tab switch tab • ? help • q quit"
		paused  = "n new • D kill │ ↵/o open • r resume │ tab switch tab • ? help • q quit"
		empty   = "n new • N new with prompt │ ? help • q quit"
		// Loading instances get the minimal option set (new/help/quit) rendered with the
		// default action-group boundary (2,5). Index 2 ("q quit") falls inside [2,5) and
		// so is rendered in the action-group style. That is pre-existing behavior, not
		// something this task changed, and it is locked in here rather than "fixed".
		loading = "n new • ? help │ q quit"
	)

	t.Run("running instance", func(t *testing.T) {
		m := NewMenu()
		m.SetSize(200, 1)
		m.SetInstance(newTestInstance(t, session.Running))
		if got := renderLine(m); got != running {
			t.Errorf("got %q, want %q", got, running)
		}
	})

	t.Run("paused instance", func(t *testing.T) {
		m := NewMenu()
		m.SetSize(200, 1)
		m.SetInstance(newTestInstance(t, session.Paused))
		if got := renderLine(m); got != paused {
			t.Errorf("got %q, want %q", got, paused)
		}
	})

	t.Run("empty state", func(t *testing.T) {
		m := NewMenu()
		m.SetSize(200, 1)
		if got := renderLine(m); got != empty {
			t.Errorf("got %q, want %q", got, empty)
		}
	})

	t.Run("loading instance", func(t *testing.T) {
		m := NewMenu()
		m.SetSize(200, 1)
		m.SetInstance(newTestInstance(t, session.Loading))
		if got := renderLine(m); got != loading {
			t.Errorf("got %q, want %q", got, loading)
		}
	})

	t.Run("instance removed reverts to the empty rendering", func(t *testing.T) {
		// Regression test for a stale actionGroupStart/actionGroupEnd surviving a
		// SetInstance(instance) -> SetInstance(nil) transition -- the real path
		// instanceChanged() drives in app/app.go when the last instance is removed
		// ("selected may be nil"). Compares against a freshly constructed menu's
		// empty-state rendering, not just the "empty" constant above, so the test
		// does not depend on that constant being correct.
		//
		// A paused instance with the preview tab active is used deliberately: of the
		// three action-group sizes addInstanceOptions can produce (5, 6 or 7), a paused
		// instance with no diff/terminal tab gives the smallest possible actionGroupEnd
		// (5), the closest any real instance state gets to the empty state's 4-item
		// option list. It is still 2 higher than the outer index (3) checked by the
		// separator loop's group-end comparison and further still from the 3 highest
		// checked indices overall, so this cannot currently make the rendering diverge
		// -- see the fix report for the full margin analysis -- but it is the tightest
		// case available through the public API, so it is what this test exercises.
		m := NewMenu()
		m.SetSize(200, 1)
		m.SetInstance(newTestInstance(t, session.Paused))
		m.SetInstance(nil)
		got := renderLine(m)

		fresh := NewMenu()
		fresh.SetSize(200, 1)
		want := renderLine(fresh)

		if got != want {
			t.Errorf("after SetInstance(nil), got %q, want %q (a freshly constructed empty menu)", got, want)
		}
		if got != empty {
			t.Errorf("got %q, want %q", got, empty)
		}
	})
}
