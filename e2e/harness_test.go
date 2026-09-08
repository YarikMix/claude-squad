//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// csBinary is the cs build under test, produced once by TestMain.
var csBinary string

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: tmux not found in PATH, skipping the package")
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "cs-build")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(1)
	}
	csBinary = filepath.Join(dir, "cs")
	build := exec.Command("go", "build", "-o", csBinary, "claude-squad")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: building cs:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

const (
	screenWidth  = 80
	screenHeight = 30
	outerSocket  = "outer"
	outerSession = "outer"
	tmuxPrefix   = "claudesquad_"
	waitTimeout  = 10 * time.Second
	pollInterval = 50 * time.Millisecond
)

// configJSON pins every setting the scenarios depend on rather than trusting defaults.
// branch_prefix is explicit because the default derives from the OS user name.
const configJSON = `{
  "default_program": "fake-agent",
  "auto_yes": false,
  "daemon_poll_interval": 1000,
  "branch_prefix": "e2e/",
  "restart_args": "--continue"
}
`

// stateJSON marks every one-time help screen as seen, so the attach and start help
// overlays do not swallow the next key a scenario sends.
const stateJSON = `{
  "help_screens_seen": 4294967295,
  "instances": []
}
`

// tmuxConf is read by both sandbox servers. default-shell must not be fake-shell: tmux
// runs every pane command through `default-shell -c`, and default-shell defaults to
// $SHELL. status off makes the outer pane exactly screenWidth×screenHeight.
const tmuxConf = "set -g default-shell /bin/sh\nset -g status off\n"

// fmtSprintf exists so a test can capture a fatal message without importing fmt twice.
var fmtSprintf = fmt.Sprintf

// pane addresses one tmux target on one sandbox server and drives it the way a user
// would: keys in, screen out.
type pane struct {
	t       *testing.T
	env     []string
	socket  string // "" for the default socket, otherwise passed as -L
	target  string // tmux target; "" for server-level commands
	timeout time.Duration
	fatalf  func(format string, args ...any) // defaults to t.Fatalf; tests override to inspect
}

func (p *pane) fail(format string, args ...any) {
	p.t.Helper()
	if p.fatalf != nil {
		p.fatalf(format, args...)
		return
	}
	p.t.Fatalf(format, args...)
}

func (p *pane) run(cmd string, rest ...string) (string, error) {
	var args []string
	if p.socket != "" {
		args = append(args, "-L", p.socket)
	}
	args = append(args, cmd)
	args = append(args, rest...)
	c := exec.Command("tmux", args...)
	c.Env = p.env
	out, err := c.CombinedOutput()
	return string(out), err
}

func (p *pane) tmux(cmd string, rest ...string) string {
	p.t.Helper()
	out, err := p.run(cmd, rest...)
	if err != nil {
		p.fail("tmux %s %s: %v\n%s", cmd, strings.Join(rest, " "), err, out)
	}
	return out
}

func (p *pane) tmuxQuiet(cmd string, rest ...string) {
	_, _ = p.run(cmd, rest...)
}

// Keys sends named keys (tmux names: Enter, Tab, C-x, Escape) or single characters.
func (p *pane) Keys(keys ...string) {
	p.t.Helper()
	p.t.Logf("keys %s -> %s", strings.Join(keys, " "), p.target)
	p.tmux("send-keys", append([]string{"-t", p.target}, keys...)...)
}

// Type sends text literally, so Cyrillic and words that collide with key names arrive as typed.
func (p *pane) Type(text string) {
	p.t.Helper()
	p.t.Logf("type %q -> %s", text, p.target)
	p.tmux("send-keys", "-t", p.target, "-l", text)
}

// Screen returns the visible pane as plain text, one line per row, trailing spaces trimmed.
func (p *pane) Screen() string {
	p.t.Helper()
	return p.tmux("capture-pane", "-p", "-t", p.target)
}

func (p *pane) waitUntil(what string, cond func() bool) {
	p.t.Helper()
	timeout := p.timeout
	if timeout == 0 {
		timeout = waitTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			screen, _ := p.run("capture-pane", "-p", "-t", p.target)
			p.fail("timed out waiting for %s\n--- screen of %s ---\n%s", what, p.target, screen)
			return
		}
		time.Sleep(pollInterval)
	}
}

// WaitFor blocks until want is on screen and returns that screen.
func (p *pane) WaitFor(want string) string {
	p.t.Helper()
	var screen string
	p.waitUntil(fmt.Sprintf("%q on screen", want), func() bool {
		screen, _ = p.run("capture-pane", "-p", "-t", p.target)
		return strings.Contains(screen, want)
	})
	return screen
}

// WaitNot blocks until want has left the screen.
func (p *pane) WaitNot(want string) {
	p.t.Helper()
	p.waitUntil(fmt.Sprintf("%q gone from screen", want), func() bool {
		screen, _ := p.run("capture-pane", "-p", "-t", p.target)
		return !strings.Contains(screen, want)
	})
}

func (p *pane) Resize(w, h int) {
	p.t.Helper()
	p.t.Logf("resize %s -> %dx%d", p.target, w, h)
	p.tmux("resize-window", "-t", p.target, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h))
}

// sessions lists the sessions on this pane's server; empty when the server is not running.
func (p *pane) sessions() []string {
	out, err := p.run("list-sessions", "-F", "#S")
	if err != nil {
		return nil
	}
	return strings.Fields(out)
}

// harness is one sandboxed cs: its own HOME, tmux servers, repository and fakes.
type harness struct {
	*pane  // the outer pane, where cs runs
	t      *testing.T
	root   string
	home   string
	repo   string // the repository cs is launched in
	origin string // bare remote of repo
	env    []string
	inner  *pane // the default-socket server, where cs creates instance sessions
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	root, err := os.MkdirTemp("", "cs")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	h := &harness{
		t:      t,
		root:   root,
		home:   filepath.Join(root, "home"),
		repo:   filepath.Join(root, "repo"),
		origin: filepath.Join(root, "origin.git"),
	}
	bin := filepath.Join(root, "bin")
	for _, d := range []string{h.home, bin, filepath.Join(root, "tmux"), filepath.Join(root, "tmp"), filepath.Join(h.home, ".claude-squad")} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	h.env = sandboxEnv(root, h.home, bin)
	for _, name := range []string{"fake-agent", "fake-shell"} {
		src, err := os.ReadFile(filepath.Join("testdata", name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), src, 0o755))
	}
	writeFile(t, filepath.Join(h.home, ".claude-squad", "config.json"), configJSON)
	writeFile(t, filepath.Join(h.home, ".claude-squad", "state.json"), stateJSON)
	writeFile(t, filepath.Join(h.home, ".tmux.conf"), tmuxConf)

	h.pane = &pane{t: t, env: h.env, socket: outerSocket, target: outerSession}
	h.inner = &pane{t: t, env: h.env}
	t.Cleanup(func() {
		if t.Failed() {
			h.dumpLog()
		}
		h.inner.tmuxQuiet("kill-server")
		h.pane.tmuxQuiet("kill-server")
	})

	h.initRepo()
	h.launch()
	return h
}

// sandboxEnv is the process environment for everything the harness starts. Every variable
// cs, tmux or git would read from the developer's machine is replaced or dropped.
func sandboxEnv(root, home, bin string) []string {
	drop := map[string]bool{
		"HOME": true, "TMUX": true, "TMUX_PANE": true, "TMUX_TMPDIR": true, "TMPDIR": true,
		"SHELL": true, "PATH": true, "XDG_CONFIG_HOME": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !drop[k] {
			env = append(env, kv)
		}
	}
	return append(env,
		"HOME="+home,
		"TMUX_TMPDIR="+filepath.Join(root, "tmux"),
		"TMPDIR="+filepath.Join(root, "tmp"), // cs logs to os.TempDir()/claudesquad.log
		"SHELL="+filepath.Join(bin, "fake-shell"),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// git runs git in dir (or with no working directory when dir is "") and fails the test on error.
func (h *harness) git(dir string, args ...string) string {
	h.t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = h.env
	out, err := c.CombinedOutput()
	require.NoError(h.t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

// initRepo builds the repository cs runs in: one commit on main, pushed to a bare origin
// so the unpushed-commit count has something to compare against.
func (h *harness) initRepo() {
	h.git("", "init", "-q", "-b", "main", h.repo)
	h.git(h.repo, "config", "user.name", "e2e")
	h.git(h.repo, "config", "user.email", "e2e@example.com")
	writeFile(h.t, filepath.Join(h.repo, "README.md"), "e2e\n")
	h.git(h.repo, "add", "README.md")
	h.git(h.repo, "commit", "-q", "-m", "init")
	h.git("", "init", "-q", "--bare", h.origin)
	h.git(h.repo, "remote", "add", "origin", h.origin)
	h.git(h.repo, "push", "-q", "origin", "main")
}

// csCommand is what the outer pane runs. TMUX and TMUX_PANE are unset so cs's tmux calls
// go to the default socket rather than the outer server, and so its nested attach is allowed.
func (h *harness) csCommand() string {
	return fmt.Sprintf("env -u TMUX -u TMUX_PANE %s -p fake-agent", csBinary)
}

func (h *harness) launch() {
	h.t.Helper()
	h.tmux("new-session", "-d", "-s", outerSession,
		"-x", strconv.Itoa(screenWidth), "-y", strconv.Itoa(screenHeight),
		"-c", h.repo, h.csCommand())
	// Keep the pane after cs exits so a scenario can relaunch cs in it.
	h.tmux("set-option", "-t", outerSession, "remain-on-exit", "on")
	h.WaitFor("Instances")
}

// dumpLog prints the tail of cs's log so a CI failure carries what cs itself reported.
func (h *harness) dumpLog() {
	data, err := os.ReadFile(filepath.Join(h.root, "tmp", "claudesquad.log"))
	if err != nil {
		h.t.Logf("no cs log: %v", err)
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	h.t.Logf("--- tail of claudesquad.log ---\n%s", strings.Join(lines, "\n"))
}

// Instance addresses the tmux session cs created for the instance with this title, on the
// inner server. cs strips whitespace from the title and prefixes it; `=` asks tmux for an
// exact match rather than a prefix match.
func (h *harness) Instance(title string) *pane {
	name := tmuxPrefix + strings.Join(strings.Fields(title), "")
	return &pane{t: h.t, env: h.env, target: "=" + name}
}

// Worktrees lists the worktree directories cs has on disk. It walks rather than globbing one
// level: branch_prefix is "e2e/", sanitizeBranchName keeps the slash, so a worktree for title
// "alpha" lands at worktrees/e2e/alpha_<hex>, two levels below the worktrees root. A directory
// is a worktree once it has a ".git" file (git worktrees never nest, so the walk stops there).
func (h *harness) Worktrees() []string {
	h.t.Helper()
	root := filepath.Join(h.home, ".claude-squad", "worktrees")
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if path == root || !d.IsDir() {
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	require.NoError(h.t, err)
	return found
}

func (h *harness) BranchExists(name string) bool {
	return strings.TrimSpace(h.git(h.repo, "branch", "--list", name)) != ""
}

// SoleWorktree returns the one worktree on disk; scenarios that need it run one instance.
func (h *harness) SoleWorktree() string {
	h.t.Helper()
	wts := h.Worktrees()
	require.Len(h.t, wts, 1, "expected exactly one worktree")
	return wts[0]
}

// CommitInWorktree writes file and commits it in dir as the agent would.
func (h *harness) CommitInWorktree(dir, file, content string) {
	h.t.Helper()
	writeFile(h.t, filepath.Join(dir, file), content)
	h.git(dir, "add", file)
	h.git(dir, "commit", "-q", "-m", "e2e: "+file)
}

// PushBranch publishes dir's branch to origin, so nothing on it counts as unpushed.
func (h *harness) PushBranch(dir string) {
	h.t.Helper()
	h.git(dir, "push", "-q", "origin", "HEAD")
}

// KillInnerServer takes down every instance session at once, the way a reboot or a stray
// `tmux kill-server` does. The outer server, and cs in it, are untouched.
func (h *harness) KillInnerServer() {
	h.t.Helper()
	h.t.Log("kill inner tmux server")
	h.inner.tmux("kill-server")
}

// Quit leaves cs through q and waits for its process to end. remain-on-exit keeps the
// outer pane so Relaunch can reuse it.
func (h *harness) Quit() {
	h.t.Helper()
	h.Keys("q")
	h.waitUntil("cs to exit", func() bool {
		out, _ := h.run("display-message", "-p", "-t", outerSession, "#{pane_dead}")
		return strings.TrimSpace(out) == "1"
	})
}

// Relaunch starts cs again in the same pane, as a user would after a reboot.
func (h *harness) Relaunch() {
	h.t.Helper()
	h.t.Log("relaunch cs")
	h.tmux("respawn-pane", "-t", outerSession, h.csCommand())
	h.WaitFor("Instances")
}

func TestHarnessStartsWithAnEmptyList(t *testing.T) {
	h := newHarness(t)
	screen := h.WaitFor("No agents running yet")
	require.Contains(t, screen, "Instances")
	require.Contains(t, screen, "Preview")
	require.Contains(t, screen, "Terminal")
	require.Empty(t, h.inner.sessions(), "cs must not create tmux sessions before the user asks for one")
}

func TestWaitForReportsTheScreenOnTimeout(t *testing.T) {
	h := newHarness(t)
	h.WaitFor("Instances")
	probe := &pane{t: t, env: h.env, socket: outerSocket, target: outerSession, timeout: 300 * time.Millisecond}
	var msg string
	probe.fatalf = func(format string, args ...any) { msg = fmtSprintf(format, args...) }
	probe.WaitFor("this text is not on screen")
	require.Contains(t, msg, "timed out waiting for")
	require.Contains(t, msg, "Instances", "the failure must carry the last screen so it reads without a rerun")
	require.True(t, strings.Contains(msg, "--- screen of outer ---"))
}
