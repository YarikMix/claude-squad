//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runFake runs one of the testdata scripts through sh with the given stdin and returns its
// combined output and exit code. It is the script's contract, checked without tmux.
func runFake(t *testing.T, name string, args []string, stdin string) (string, int) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{filepath.Join("testdata", name)}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}
	return out.String(), code
}

func TestFakeAgentBannerCarriesArgs(t *testing.T) {
	out, code := runFake(t, "fake-agent", nil, "")
	require.Equal(t, 0, code)
	require.Contains(t, out, "fake-agent ready args=[]")

	out, _ = runFake(t, "fake-agent", []string{"--continue"}, "")
	require.Contains(t, out, "fake-agent ready args=[--continue]")
}

func TestFakeAgentCommands(t *testing.T) {
	out, code := runFake(t, "fake-agent", nil, "echo hello there\nwide 5\nexit 3\necho never\n")
	require.Equal(t, 3, code)
	require.Contains(t, out, "fake-agent: hello there\n")
	require.Contains(t, out, "\nxxxxx\n")
	require.NotContains(t, out, "never")
}

func TestFakeShellPromptShape(t *testing.T) {
	out, code := runFake(t, "fake-shell", nil, "ls\n")
	require.Equal(t, 0, code)
	// The prompt is wider than any test pane and ends in spaces broken by an SGR reset:
	// the shape a powerlevel10k right prompt leaves in a tmux capture (PR #13).
	require.Contains(t, out, strings.Repeat("=", 100)+"        \x1b[0m        \n")
	require.Contains(t, out, "fake-shell$ ")
	require.Contains(t, out, "fake-shell: ls\n")
	require.Equal(t, 2, strings.Count(out, "fake-shell$ "), "a prompt before and after the command")
}
