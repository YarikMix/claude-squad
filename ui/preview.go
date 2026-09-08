package ui

import (
	"claude-squad/session"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var previewPaneStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

type PreviewPane struct {
	width  int
	height int

	previewState previewState
	isScrolling  bool
	viewport     viewport.Model
}

type previewState struct {
	// fallback is true if the preview pane is displaying fallback text
	fallback bool
	// text is the text displayed in the preview pane
	text string
}

func NewPreviewPane() *PreviewPane {
	return &PreviewPane{
		viewport: viewport.New(0, 0),
	}
}

func (p *PreviewPane) SetSize(width, maxHeight int) {
	p.width = width
	p.height = maxHeight
	p.viewport.Width = width
	p.viewport.Height = maxHeight
}

// setFallbackState sets the preview state with fallback text and a message
func (p *PreviewPane) setFallbackState(message string) {
	// Word-wrap and center the message to the pane width before joining it below the banner and
	// clamping with fitBox: a message line can be wider than the pane (e.g. the "checked out at"
	// branch hint), and wrapping it now, rather than letting JoinVertical center it against its
	// own widest line first, keeps every line readable instead of clipped by fitBox. This is safe
	// because the fallback text carries no captured escape sequences, unlike pane content — see
	// fitBox's doc comment.
	wrapped := message
	if p.width > 0 {
		wrapped = lipgloss.NewStyle().Width(p.width).Align(lipgloss.Center).Render(message)
	}

	text := wrapped
	if p.width == 0 || p.width >= lipgloss.Width(FallBackText) {
		// The banner fits in the pane (or the pane size isn't known yet); show it as before.
		text = lipgloss.JoinVertical(lipgloss.Center, FallBackText, "", wrapped)
	}

	p.previewState = previewState{
		fallback: true,
		text:     text,
	}
}

// Updates the preview pane content with the tmux pane content
func (p *PreviewPane) UpdateContent(instance *session.Instance) error {
	switch {
	case instance == nil:
		p.setFallbackState("No agents running yet. Spin up a new instance with 'n' to get started!")
		return nil
	case instance.Status == session.Loading:
		p.setFallbackState("Setting up workspace...")
		return nil
	case instance.Status == session.Paused:
		// Join with plain newlines, not lipgloss.JoinVertical: JoinVertical would center each
		// line against the block's own widest line (the branch hint below) before setFallbackState
		// ever sees the pane width, baking in padding that wrapping afterward can't undo.
		p.setFallbackState(strings.Join([]string{
			"Session is paused. Press 'r' to resume.",
			"",
			lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{
					Light: "#FFD700",
					Dark:  "#FFD700",
				}).
				Render(fmt.Sprintf(
					"The instance can be checked out at '%s' (copied to your clipboard)",
					instance.Branch,
				)),
		}, "\n"))
		return nil
	}

	var content string
	var err error

	// If in scroll mode but haven't captured content yet, do it now
	if p.isScrolling && p.viewport.Height > 0 && len(p.viewport.View()) == 0 {
		// Capture full pane content including scrollback history using capture-pane -p -S -
		content, err = instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}).
			Render("ESC to exit scroll mode")

		p.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, content, footer))
	} else if !p.isScrolling {
		// In normal mode, use the usual preview
		content, err = instance.Preview()
		if err != nil {
			return err
		}

		// Always update the preview state with content, even if empty
		// This ensures that newly created instances will display their content immediately
		if len(content) == 0 && !instance.Started() {
			p.setFallbackState("Please enter a name for the instance.")
		} else {
			// Update the preview state with the current content
			p.previewState = previewState{
				fallback: false,
				text:     content,
			}
		}
	}

	return nil
}

// Returns the preview pane content as a string.
func (p *PreviewPane) String() string {
	if p.width == 0 || p.height == 0 {
		return strings.Repeat("\n", p.height)
	}

	if p.previewState.fallback {
		// Place, then clamp — never wrap here. p.previewState.text is already sized to fit
		// p.width by setFallbackState (the ASCII banner is one long run per line with no
		// whitespace to break on, so it is kept unwrapped and dropped instead once the pane is
		// narrower than it; the message part was already word-wrapped and centered to p.width).
		// Place only positions and pads vertically, it never re-wraps, and fitBox truncates
		// whatever still overflows the box. See fitBox's own doc comment.
		placed := lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, p.previewState.text)
		return previewPaneStyle.Render(fitBox(placed, p.width, p.height, false, ""))
	}

	// If in copy mode, use the viewport to display scrollable content
	if p.isScrolling {
		return p.viewport.View()
	}

	// Normal mode display
	// Calculate available height accounting for border and margin
	availableHeight := p.height - 1 //  1 for ellipsis

	// Strip OSC sequences first: their invisible payload is counted as visible text by the
	// width helpers below, which would make this pane report itself wider than it is.
	//
	// Render before clamping: the style wraps overlong lines, so a block clamped first would
	// grow back past the pane height. See fitBox. Preview reads from the top, so an
	// overlong capture keeps its head and marks the cut.
	rendered := previewPaneStyle.Width(p.width).Render(stripOSCSequences(p.previewState.text))
	return fitBox(rendered, p.width, availableHeight, false, "...")
}

// ScrollUp scrolls up in the viewport
func (p *PreviewPane) ScrollUp(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if !p.isScrolling {
		// Entering scroll mode - capture entire pane content including scrollback history
		content, err := instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}).
			Render("ESC to exit scroll mode")

		contentWithFooter := lipgloss.JoinVertical(lipgloss.Left, content, footer)
		p.viewport.SetContent(contentWithFooter)

		// Position the viewport at the bottom initially
		p.viewport.GotoBottom()

		p.isScrolling = true
		return nil
	}

	// Already in scroll mode, just scroll the viewport
	p.viewport.LineUp(1)
	return nil
}

// ScrollDown scrolls down in the viewport
func (p *PreviewPane) ScrollDown(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if !p.isScrolling {
		// Entering scroll mode - capture entire pane content including scrollback history
		content, err := instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}).
			Render("ESC to exit scroll mode")

		contentWithFooter := lipgloss.JoinVertical(lipgloss.Left, content, footer)
		p.viewport.SetContent(contentWithFooter)

		// Position the viewport at the bottom initially
		p.viewport.GotoBottom()

		p.isScrolling = true
		return nil
	}

	// Already in copy mode, just scroll the viewport
	p.viewport.LineDown(1)
	return nil
}

// ResetToNormalMode exits scroll mode and returns to normal mode
func (p *PreviewPane) ResetToNormalMode(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if p.isScrolling {
		p.isScrolling = false
		// Reset viewport
		p.viewport.SetContent("")
		p.viewport.GotoTop()

		// Immediately update content instead of waiting for next UpdateContent call
		content, err := instance.Preview()
		if err != nil {
			return err
		}
		p.previewState.text = content
	}

	return nil
}
