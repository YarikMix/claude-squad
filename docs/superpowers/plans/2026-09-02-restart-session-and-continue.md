# Restart Session and Continue Conversation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring an agent's conversation back when its session's process is restarted or
rebuilt, and add a hotkey that restarts the agent without killing the session, its worktree,
or its branch.

**Architecture:** A configurable `restart_args` string is appended to the session's program
to form a *restart command*, used in exactly two places: `tmux respawn-pane -k` (the new
restart hotkey) and the `tmux new-session` issued by `Resume` when the old tmux session is
gone. A brand new session never gets the args, because a fresh worktree has no conversation
to continue. The restart command carries a shell `|| <program>` fallback so that an agent
which exits non-zero on its resume flag cannot take the pane — and with it the tmux
session — down.

**Tech Stack:** Go 1.23.0 (toolchain go1.25.8), tmux CLI, bubbletea/lipgloss TUI,
testify/require, `cmd.Executor` + `PtyFactory` seams for mocking tmux invocations.

**Spec:** `docs/superpowers/specs/2026-09-02-restart-session-and-continue-design.md`

## Provenance of the code in this plan

The code blocks below are not sketches: they come from a prototype that was built, compiled,
vetted and run against the full test suite earlier in this session, then reverted so the work
could be redone plan-first. They are known to compile and pass.

That does **not** make the TDD ordering optional. In the prototype the tests were written
*after* the implementation, so no test was ever observed failing — which means none of them
was ever proven to test anything. Step 2 of each task ("run the test, watch it fail") is the
step that earns the tests their credibility, and it is the main thing this pass adds over the
prototype. Do not skip it, and do not paste implementation and tests together.

## Prerequisites

- [ ] **Create the delivery branch before Task 1.** The spec's delivery section requires it,
  and every task below commits, so this cannot wait until the end.

```bash
git checkout -b feature/restart-and-continue
git status --short   # expect: only untracked docs/
```

- [ ] **Confirm a green baseline.**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: all packages pass. If they do not, stop — the failure is pre-existing and unrelated
to this work.

## Global Constraints

- Fork of `smtg-ai/claude-squad` v1.0.20 (`ce1ffb4`); the fork is `YarikMix/claude-squad`.
- Go toolchain `go1.25.8`, module `claude-squad`, Go directive `1.23.0`.
- Restart args are read from the config, **never** persisted to `state.json` — a config edit
  must affect existing instances without a state migration.
- Restart args **augment** the program, never replace it: `default_program` may carry its own
  flags (e.g. `claude --add-dir ~/projects/cloud/cloud.mail.ru`) which must survive a restart.
- Restart args are **never** applied on a session's first start.
- Restart must not touch the tmux session's identity, the git worktree, or the branch.
- Keys: `R` in the instance list, `Ctrl+X` (ASCII 24) inside an attached session.
- Delivery: branch `feature/restart-and-continue`; build with `go build -o ~/.local/bin/cs .`;
  `~/.local/bin` must precede `/opt/homebrew/bin` in `PATH` (verified: positions 4 and 12).
- Upstream framing: two PRs, presented as **features**, not as a fix. "Restore `--continue`"
  is a false framing — upstream never had it (`git log --all -S '--continue'` is empty across
  all 222 commits, 21 tags and every branch).

## Binding decisions from the user

The spec left three questions open. The user answered them; the answers are requirements, not
suggestions, and each names the task that implements it.

| Question | Decision | Task |
|---|---|---|
| Restart in a worktree with no conversation on disk, where `claude --continue` exits non-zero and would close the pane | **Shell fallback:** compose the command as `<program> <args> \|\| <program>`, so a failed resume falls through to a clean start and the pane survives. No probing of `~/.claude/projects/`, so the fork stays independent of Claude Code's storage format. | 2 |
| Where `Ctrl+X` should be live | **Only sessions of the agent.** Gate the interception on a non-empty restart command. The Terminal tab builds its own `TmuxSession` and never sets one, so `Ctrl+X` keeps reaching that shell and the programs inside it. | 5 |
| Confirmation prompt on `R` | **No confirmation**, as the spec has it. `R` acts immediately. | 4 |

## Corrections to the spec

Each was verified, not assumed. Carry them into the code comments where noted.

1. **`respawn-pane` does not preserve scrollback.** The spec claims it does. Verified against
   real tmux: after `respawn-pane -k`, `capture-pane -p -S - -E -` finds nothing from before
   the respawn. What does survive is the session, its name, the PTY and the attached
   connection. The conversation comes back because the agent redraws it from `--continue`,
   not because tmux kept the buffer. `RespawnPane`'s doc comment must say so (Task 2).
2. **Existing configs have no `restart_args` key.** The spec does not mention it. A current
   `~/.claude-squad/config.json` predates the key, so it unmarshals to `""` and the feature
   would be silently off for every existing user — including the user who requested it. An
   absent key must be treated as "never configured" and backfilled; an explicit `""` is the
   documented opt-out and must be left alone (Task 1).
3. **`ui/menu.go` hardcodes menu group boundaries** as `{0,2}, {2,5}, {6,8}`, used both to
   highlight the action group and to place the `│` separators. Appending a menu entry shifts
   every later index and silently breaks both. The boundaries must be computed (Task 6).

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `config/config.go` | `restart_args` key, its default, backfill for pre-existing configs | Modify |
| `config/config_test.go` | Default, explicit value, backfill, opt-out | Modify |
| `session/tmux/tmux.go` | Restart command composition, `RespawnPane`, restart-aware start, `Ctrl+X` | Modify |
| `session/tmux/tmux_test.go` | Command strings for respawn/start, fallback, missing session | Modify |
| `session/instance.go` | Reads config, sets the restart command, `Restart()`, restart-aware `Resume()` | Modify |
| `session/instance_test.go` | `Restart()` guards and the respawn it issues | Modify |
| `keys/keys.go` | `KeyRestart` bound to `R` | Modify |
| `app/app.go` | `KeyRestart` handler | Modify |
| `ui/menu.go` | `R restart` entry; computed group boundaries | Modify |
| `app/help.go` | `R` and `ctrl-x` in the help screens | Modify |
| `README.md` | Keybindings and `restart_args` documentation | Modify |

No new files. The change is a thin capability threaded through existing layers; splitting it
across new files would scatter it without reducing any file's responsibility.

---

### Task 1: Config key `restart_args`

**Files:**
- Modify: `config/config.go` — const block, `Config` struct, `DefaultConfig`, `LoadConfig`
- Test: `config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Config.RestartArgs string` (JSON key `restart_args`), default `"--continue"`.
  Task 3 reads it as `config.LoadConfig().RestartArgs`.

- [ ] **Step 1: Write the failing tests**

Append to `config/config_test.go`. The file already has `TestMain`, `assert` and `require`
imported, and `filepath`/`os` available.

```go
func TestRestartArgs(t *testing.T) {
	t.Run("default is --continue, matching the default program", func(t *testing.T) {
		assert.Equal(t, "--continue", DefaultConfig().RestartArgs)
	})

	t.Run("reads the configured value", func(t *testing.T) {
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".claude-squad")
		require.NoError(t, os.MkdirAll(configDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, ConfigFileName),
			[]byte(`{"default_program": "aider", "restart_args": "--restore-chat-history"}`), 0644))

		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempHome)
		defer os.Setenv("HOME", originalHome)

		assert.Equal(t, "--restore-chat-history", LoadConfig().RestartArgs)
	})

	// Config files written before restart_args existed have no such key. Leaving it at ""
	// would silently disable restarting with a resumed conversation for every existing
	// user, so an absent key is backfilled with the default and persisted.
	t.Run("backfills the default when the key is absent", func(t *testing.T) {
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".claude-squad")
		require.NoError(t, os.MkdirAll(configDir, 0755))
		configPath := filepath.Join(configDir, ConfigFileName)
		require.NoError(t, os.WriteFile(configPath,
			[]byte(`{"default_program": "claude", "auto_yes": false}`), 0644))

		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempHome)
		defer os.Setenv("HOME", originalHome)

		assert.Equal(t, "--continue", LoadConfig().RestartArgs)

		// The backfill is persisted, so the user can see and edit the key.
		written, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(written), `"restart_args": "--continue"`)
		assert.Contains(t, string(written), `"default_program": "claude"`,
			"backfilling must not drop the rest of the config")
	})

	// An explicit "" is the documented way to opt out; it must not be overwritten.
	t.Run("keeps an explicit empty value", func(t *testing.T) {
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".claude-squad")
		require.NoError(t, os.MkdirAll(configDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, ConfigFileName),
			[]byte(`{"default_program": "claude", "restart_args": ""}`), 0644))

		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempHome)
		defer os.Setenv("HOME", originalHome)

		assert.Equal(t, "", LoadConfig().RestartArgs)
	})
}
```

- [ ] **Step 2: Run the tests and watch them fail**

```bash
go test ./config/ -run TestRestartArgs
```

Expected: compile failure — `config.RestartArgs` undefined. A compile failure is a valid
"fails first" here: the field genuinely does not exist yet.

- [ ] **Step 3: Add the field, its default, and the backfill**

In the const block next to `defaultProgram`:

```go
	// defaultRestartArgs matches defaultProgram: `claude --continue` resumes the most
	// recent conversation in the worktree. Users of other agents override it.
	defaultRestartArgs = "--continue"
```

In `Config`, after `Profiles`:

```go
	// RestartArgs are appended to the program when a session's process is restarted or
	// when a session is resumed after its tmux session died. They are never used on the
	// first start, where a fresh worktree has no conversation to continue. Applied
	// verbatim; set to "" for programs that have no resume flag.
	RestartArgs string `json:"restart_args"`
```

In `DefaultConfig()`'s returned literal, after `DaemonPollInterval`:

```go
		RestartArgs:        defaultRestartArgs,
```

In `LoadConfig()`, between the successful `json.Unmarshal` into `config` and `return &config`:

```go
	// Config files written before restart_args existed have no such key, which would
	// unmarshal to "" and silently disable restarting with a resumed conversation. An
	// absent key means "never configured", so backfill the default and persist it; an
	// explicit "" means the user opted out and is left alone.
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err == nil {
		if _, ok := present["restart_args"]; !ok {
			config.RestartArgs = defaultRestartArgs
			if saveErr := saveConfig(&config); saveErr != nil {
				log.WarningLog.Printf("failed to backfill restart_args in config: %v", saveErr)
			}
		}
	}
```

- [ ] **Step 4: Run the tests and watch them pass**

```bash
go test ./config/
```

Expected: PASS, all four subtests. `TestSaveConfig` must still pass: `saveConfig` always
emits the key, so a round-trip through it is never mistaken for an absent key.

- [ ] **Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat(config): add restart_args, appended to the program on restart"
```

---

### Task 2: Restart command in the tmux layer

**Files:**
- Modify: `session/tmux/tmux.go` — `TmuxSession` struct, `Start`, plus new functions
- Test: `session/tmux/tmux_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 directly; the args arrive as a plain string.
- Produces, all relied on by Tasks 3 and 5:
  - `func BuildRestartCommand(program, args string) string`
  - `func (t *TmuxSession) SetRestartCommand(command string)`
  - `func (t *TmuxSession) RespawnPane() error` — returns `ErrSessionNotFound` when the session is gone
  - `func (t *TmuxSession) StartWithRestartCommand(workDir string) error`
  - unexported `restartCommandOrProgram() string`, and the field `restartCommand string`

**Why a single-string command with `||` works:** tmux hands a *single* shell-command argument
to the shell, so `claude --continue || claude` is evaluated as a shell expression. This is the
same mechanism the existing code already depends on — `Start` passes `t.program` as one
argument, which is why a `default_program` of `claude --add-dir ~/foo` works at all. Verified
against real tmux both ways: with the fallback, a failed resume starts a clean agent and the
session survives; without it, the same respawn kills the session.

- [ ] **Step 1: Write the failing tests**

Append to `session/tmux/tmux_test.go`. It already provides `MockPtyFactory`,
`cmd_test.MockCmdExec`, `cmd2.ToString` and `newTmuxSession`; no new scaffolding is needed.

```go
func TestBuildRestartCommand(t *testing.T) {
	// tmux hands a single shell-command argument to the shell, so the `||` fallback is
	// honored. Without it, `claude --continue` in a worktree with no conversation on disk
	// exits non-zero and takes the pane — and the whole tmux session — down with it.
	require.Equal(t, "claude --continue || claude", BuildRestartCommand("claude", "--continue"))

	// Flags already in the program must survive the restart.
	require.Equal(t,
		"claude --add-dir ~/projects/foo --continue || claude --add-dir ~/projects/foo",
		BuildRestartCommand("claude --add-dir ~/projects/foo", "--continue"))

	// Programs with no resume flag restart as-is.
	require.Equal(t, "aider", BuildRestartCommand("aider", ""))
	require.Equal(t, "aider", BuildRestartCommand("aider", "   "))
	require.Equal(t, "", BuildRestartCommand("", "--continue"))
}

func TestRespawnPaneUsesRestartCommand(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd2.ToString(cmd))
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("restarted", "claude", NewMockPtyFactory(t), cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.NoError(t, session.RespawnPane())
	require.Contains(t, ran,
		"tmux respawn-pane -k -t claudesquad_restarted claude --continue || claude")
}

// With no restart command configured, a restart still has to bring the program back up.
func TestRespawnPaneFallsBackToProgram(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ran = append(ran, cmd2.ToString(cmd))
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("plain", "aider", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, session.RespawnPane())
	require.Contains(t, ran, "tmux respawn-pane -k -t claudesquad_plain aider")
}

// Respawning a pane of a session that no longer exists cannot work; callers need to tell
// that apart from a failed respawn so they can point the user at resume instead.
func TestRespawnPaneReturnsErrSessionNotFoundWhenSessionIsGone(t *testing.T) {
	respawned := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			if strings.Contains(cmd.String(), "respawn-pane") {
				respawned = true
			}
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("gone", "claude", NewMockPtyFactory(t), cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.ErrorIs(t, session.RespawnPane(), ErrSessionNotFound)
	require.False(t, respawned, "should not respawn a pane of a session that does not exist")
}

// Resume rebuilds a session whose tmux server died. That new session is where the
// conversation is lost today, so it starts the restart command rather than the bare program.
func TestStartWithRestartCommandStartsRestartCommand(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("output"), nil },
	}

	workdir := t.TempDir()
	session := newTmuxSession("resumed", "claude", ptyFactory, cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.NoError(t, session.StartWithRestartCommand(workdir))
	require.Equal(t, fmt.Sprintf(
		"tmux new-session -d -s claudesquad_resumed -c %s claude --continue || claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}

// A brand new session must not get the restart args: a fresh worktree has no conversation,
// and `claude --continue` there would fail on the very first start.
func TestStartIgnoresRestartCommand(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("output"), nil },
	}

	workdir := t.TempDir()
	session := newTmuxSession("fresh", "claude", ptyFactory, cmdExec)
	session.SetRestartCommand("claude --continue || claude")

	require.NoError(t, session.Start(workdir))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s claudesquad_fresh -c %s claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}
```

- [ ] **Step 2: Run the tests and watch them fail**

```bash
go test ./session/tmux/
```

Expected: compile failure — `BuildRestartCommand`, `SetRestartCommand`, `RespawnPane` and
`StartWithRestartCommand` undefined.

- [ ] **Step 3: Implement**

Add the field to `TmuxSession`, right after `program`:

```go
	// restartCommand is the command used to bring the program back up on restart. It is
	// empty for sessions that should not be restartable (the terminal tab's shell), in
	// which case restarts fall back to the bare program.
	restartCommand string
```

Add, after `newTmuxSession`:

```go
// SetRestartCommand sets the command used by RespawnPane and StartWithRestartCommand.
// Setting a non-empty value also enables the in-session Ctrl+x restart shortcut.
func (t *TmuxSession) SetRestartCommand(command string) {
	t.restartCommand = command
}

// BuildRestartCommand composes the command that brings program back up with args appended,
// falling back to the bare program if the args fail. tmux passes a single shell-command
// argument to the shell, so the `||` is honored: `claude --continue` exits non-zero in a
// worktree with no conversation on disk, and without the fallback that would take the pane
// — and with it the tmux session — down with it.
//
// Returns program unchanged when args is empty, so programs with no resume flag still
// restart normally.
func BuildRestartCommand(program, args string) string {
	program = strings.TrimSpace(program)
	args = strings.TrimSpace(args)
	if program == "" || args == "" {
		return program
	}
	return fmt.Sprintf("%s %s || %s", program, args, program)
}

// restartCommandOrProgram returns the configured restart command, or the bare program when
// none is set.
func (t *TmuxSession) restartCommandOrProgram() string {
	if t.restartCommand == "" {
		return t.program
	}
	return t.restartCommand
}

// RespawnPane restarts the program in the session's pane, replacing the running process.
// The tmux session, its name and the attached PTY survive, so a user attached to the pane
// stays attached and watches the program come back up. The pane's scrollback does not
// survive: tmux clears it on respawn.
func (t *TmuxSession) RespawnPane() error {
	if !t.DoesSessionExist() {
		return ErrSessionNotFound
	}
	cmd := exec.Command("tmux", "respawn-pane", "-k", "-t", t.sanitizedName, t.restartCommandOrProgram())
	if err := t.cmdExec.Run(cmd); err != nil {
		return fmt.Errorf("error respawning tmux pane for session %s: %w", t.sanitizedName, err)
	}
	return nil
}
```

Split `Start` so the command becomes a parameter. Keep the existing body verbatim in `start`,
changing only the `new-session` invocation's last argument from `t.program` to `command`:

```go
// Start creates and starts a new tmux session, then attaches to it. Program is the command to run in
// the session (ex. claude). workdir is the git worktree directory.
func (t *TmuxSession) Start(workDir string) error {
	return t.start(workDir, t.program)
}

// StartWithRestartCommand creates the session running the restart command rather than the
// bare program, so a session rebuilt after its tmux server died comes back with the
// agent's previous conversation. Falls back to the program when no restart command is set.
func (t *TmuxSession) StartWithRestartCommand(workDir string) error {
	return t.start(workDir, t.restartCommandOrProgram())
}

func (t *TmuxSession) start(workDir string, command string) error {
	// Check if the session already exists
	if t.DoesSessionExist() {
		return fmt.Errorf("tmux session already exists: %s", t.sanitizedName)
	}

	// Create a new detached tmux session and start claude in it
	cmd := exec.Command("tmux", "new-session", "-d", "-s", t.sanitizedName, "-c", workDir, command)

	// ... rest of the existing Start body, unchanged ...
}
```

- [ ] **Step 4: Run the tests and watch them pass**

```bash
go test ./session/tmux/
```

Expected: PASS, including the pre-existing `TestStartTmuxSession`, which asserts
`Start` still emits `tmux new-session -d -s ... -c <dir> claude` with nothing appended. That
test is the regression guard for "never on first start".

- [ ] **Step 5: Commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_test.go
git commit -m "feat(tmux): add RespawnPane and a restart command with a shell fallback"
```

---

### Task 3: `Instance.Restart()` and a restart-aware `Resume()`

**Files:**
- Modify: `session/instance.go` — imports, `FromInstanceData`, `Start`, `Resume`, new `Restart`
- Test: `session/instance_test.go`

**Interfaces:**
- Consumes: `config.Config.RestartArgs` (Task 1); `tmux.BuildRestartCommand`,
  `SetRestartCommand`, `RespawnPane`, `StartWithRestartCommand` (Task 2).
- Produces: `func (i *Instance) Restart() error`, called by Task 4, and the package-level test
  seam `var loadRestartArgs func() string`.

**Import note:** `session` already imports `config` in `session/storage.go`, so this adds no
new dependency and creates no cycle — `config` imports only `log`.

**Why a test seam:** without it, `Start` would read the developer's real
`~/.claude-squad/config.json` during tests, and would *create* one on a machine that has
none. Pinning `loadRestartArgs` in the package `TestMain` keeps the suite hermetic.

- [ ] **Step 1: Write the failing tests**

First pin the args in the existing `TestMain` in `session/instance_test.go`:

```go
func TestMain(m *testing.M) {
	log.Initialize(false)
	defer log.Close()
	// Pin the restart args so the suite neither reads nor creates the developer's real
	// config file. Individual tests override this where the value matters.
	loadRestartArgs = func() string { return "--continue" }
	os.Exit(m.Run())
}
```

Then append the tests. `nullPtyFactory` and `cmd_test.MockCmdExec` already exist in the file;
`fmt` and `strings` are already imported.

```go
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
```

- [ ] **Step 2: Run the tests and watch them fail**

```bash
go test ./session/ -run TestRestart
```

Expected: compile failure — `Restart` and `loadRestartArgs` undefined.

- [ ] **Step 3: Implement**

Add `"claude-squad/config"` to the imports, then, above `RepoName`:

```go
// loadRestartArgs reads the restart args from the config. It is a variable so tests can pin
// a value instead of depending on (and creating) the user's real config file.
var loadRestartArgs = func() string {
	return config.LoadConfig().RestartArgs
}

// restartCommandFor builds the command used to bring program back up on restart. The args
// are read from the config on every call rather than stored in state.json, so editing the
// config takes effect on existing instances without migrating their saved state.
func restartCommandFor(program string) string {
	return tmux.BuildRestartCommand(program, loadRestartArgs())
}
```

Set the command at both session-creation sites. In `FromInstanceData`'s paused branch, after
the `NewTmuxSession` call:

```go
		instance.tmuxSession.SetRestartCommand(restartCommandFor(instance.Program))
```

In `Start`, right after `i.tmuxSession = tmuxSession`:

```go
	// Restart args are never used for the session created below: a fresh worktree has no
	// conversation to continue. They apply to Restart and to Resume, which rebuilds a
	// session whose tmux server died.
	i.tmuxSession.SetRestartCommand(restartCommandFor(i.Program))
```

In `Resume`, switch the two calls that *create* a session from `Start` to
`StartWithRestartCommand` — the one in the `Restore` failure fallback and the one in the
`else` branch. Leave the plain `Restore` alone and note why:

```go
	// Check if tmux session still exists from pause, otherwise create new one. A session
	// that survived the pause still has the program running with its context, so only the
	// paths that create a new session ask for the restart command.
```

Add the new method after `Resume`:

```go
// Restart replaces the program running in the session's pane, appending the configured
// restart args so the agent comes back with its previous conversation. The tmux session,
// the git worktree and the branch are left untouched, and a user attached to the pane stays
// attached.
func (i *Instance) Restart() error {
	if !i.started {
		return fmt.Errorf("cannot restart instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("cannot restart a paused session: press 'r' to resume it first")
	}
	// Respawning a pane of a session that no longer exists cannot work. Point the user at
	// Resume, which rebuilds the session from the branch.
	if !i.tmuxSession.DoesSessionExist() {
		return fmt.Errorf("tmux session for '%s' no longer exists: press 'r' to resume it", i.Title)
	}
	// Re-read the args so a config edit applies without restarting claude-squad.
	i.tmuxSession.SetRestartCommand(restartCommandFor(i.Program))
	if err := i.tmuxSession.RespawnPane(); err != nil {
		return fmt.Errorf("failed to restart session '%s': %w", i.Title, err)
	}
	i.SetStatus(Running)
	return nil
}
```

- [ ] **Step 4: Run the tests and watch them pass**

```bash
go test ./session/...
```

Expected: PASS, including the two pre-existing tests for a tmux server that died
(`TestStartPausesInstanceWhenTmuxSessionNoLongerExists`,
`TestStartRestoresInstanceWhenTmuxSessionSurvives`).

- [ ] **Step 5: Commit**

```bash
git add session/instance.go session/instance_test.go
git commit -m "feat(session): add Instance.Restart and resume with the restart command"
```

---

### Task 4: `R` hotkey in the instance list

**Files:**
- Modify: `keys/keys.go` — `KeyName` enum, `GlobalKeyStringsMap`, `GlobalkeyBindings`
- Modify: `app/app.go` — the global `switch name`, next to `case keys.KeyResume`
- Test: none automated. `app` has no harness for the key switch; Step 3 is a read-and-verify.

**Interfaces:**
- Consumes: `Instance.Restart()` (Task 3).
- Produces: `keys.KeyRestart`, bound to `"R"` with help text `R restart`. Task 6 renders it.

- [ ] **Step 1: Add the key**

`KeyRestart` at the end of the const block — the values are map keys only, never persisted,
so appending renumbers nothing that matters:

```go
	// KeyRestart restarts the program running in the session's pane.
	KeyRestart
```

`"R": KeyRestart` in `GlobalKeyStringsMap`, after `"r": KeyResume`, and the binding after
`KeyResume`'s:

```go
	KeyRestart: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "restart"),
	),
```

- [ ] **Step 2: Handle it**

In `app/app.go`, immediately after the `case keys.KeyResume:` block. No confirmation modal,
per the user's decision:

```go
	case keys.KeyRestart:
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Status == session.Loading {
			return m, nil
		}
		if err := selected.Restart(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
```

- [ ] **Step 3: Verify `R` cannot leak into text entry**

Read `handleKeyPress` in `app/app.go` and confirm that `stateNew`, `statePrompt`, `stateHelp`
and `stateConfirm` are all dispatched *before* control reaches the global `switch`, so typing
`R` in a session name or a prompt is unaffected. `"R"` was previously unmapped, so there is no
existing binding to collide with.

```bash
go build ./... && go vet ./...
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add keys/keys.go app/app.go
git commit -m "feat(keys): bind R to restarting the selected session"
```

---

### Task 5: `Ctrl+X` inside an attached session

**Files:**
- Modify: `session/tmux/tmux.go` — `Attach`'s stdin loop, next to the `Ctrl+Q` byte check
- Test: none automated. The loop reads real `os.Stdin`; the spec says so and that stands.

**Interfaces:**
- Consumes: `RespawnPane` and the `restartCommand` field (Task 2).
- Produces: nothing for later tasks.

**Gating, per the user's decision:** the interception is conditional on
`t.restartCommand != ""`. `ui/terminal.go` builds its own `TmuxSession` for the Terminal tab
and never calls `SetRestartCommand`, so there `Ctrl+X` keeps reaching the shell and whatever
runs inside it. That emptiness is the only thing distinguishing agent sessions from terminal
sessions here, which is why Task 3 must set the command on every agent session — including
paused ones, via `FromInstanceData`.

- [ ] **Step 1: Intercept the byte**

Immediately after the existing `nr == 1 && buf[0] == 17` check:

```go
			// Check for Ctrl+x (ASCII 24): restart the program in the pane without
			// tearing down the attached connection. Only sessions with a restart command
			// take part; the terminal tab leaves it empty so Ctrl+x reaches its shell.
			if nr == 1 && buf[0] == 24 && t.restartCommand != "" {
				if err := t.RespawnPane(); err != nil {
					log.ErrorLog.Printf("error restarting pane for session %s: %v", t.sanitizedName, err)
				}
				continue
			}
```

`continue`, not `return`: the connection must survive and the loop must keep forwarding
input. Errors go to the log because there is no UI to surface them to from inside an attached
pane.

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: clean, all packages pass.

- [ ] **Step 2: Verify the fallback against real tmux**

Not a unit test — a scripted check of the exact behavior the `||` exists for. Write a stub
agent that exits 1 on `--continue` unless a history file is present:

```bash
cat > /tmp/fakeclaude <<'EOF'
#!/bin/sh
MARKER="$(pwd)/.history"
if [ "$1" = "--continue" ]; then
  [ -f "$MARKER" ] || { echo "No conversation found to continue"; exit 1; }
  echo "AGENT_UP: CONTINUED_CONVERSATION"
else
  echo "AGENT_UP: FRESH_START"; touch "$MARKER"
fi
sleep 300
EOF
chmod +x /tmp/fakeclaude
mkdir -p /tmp/wt && rm -f /tmp/wt/.history
export PATH=/tmp:$PATH

# No conversation on disk: the fallback must keep the session alive.
tmux new-session -d -s itest -c /tmp/wt "sleep 300"
tmux respawn-pane -k -t itest "fakeclaude --continue || fakeclaude"
sleep 1
tmux capture-pane -p -t itest      # expect: "No conversation found" then "FRESH_START"
tmux has-session -t=itest          # expect: exit 0

# Contrast: without the fallback the session dies.
tmux respawn-pane -k -t itest "sleep 300"; rm -f /tmp/wt/.history
tmux respawn-pane -k -t itest "fakeclaude --continue"
sleep 1
tmux has-session -t=itest          # expect: non-zero — this is the footgun
tmux kill-session -t itest 2>/dev/null
```

Expected: the first case leaves the session alive with a fresh agent; the second kills it.
If the first case kills the session, the `||` is not reaching the shell — stop and fix Task 2
rather than working around it here.

- [ ] **Step 3: Manual verification in the real TUI**

Requires a running `cs` and a human at the keyboard. Build and install first (this is also
the spec's delivery step, so do it here rather than duplicating it later):

```bash
go build -o ~/.local/bin/cs .
```

Then attach to a session and confirm:
- `Ctrl+X` brings the agent back with the previous conversation.
- The connection is not dropped and no red `Session terminated without detaching` appears.
- `Ctrl+X` in the Terminal tab is forwarded to the shell instead of respawning it.

- [ ] **Step 4: Commit**

```bash
git add session/tmux/tmux.go
git commit -m "feat(tmux): restart the pane's program on Ctrl+X while attached"
```

---

### Task 6: Menu entry, help screens, README

**Files:**
- Modify: `ui/menu.go` — `Menu` struct, `NewMenu`, `addInstanceOptions`, `String`
- Modify: `app/help.go` — the general help and the attach help screen
- Modify: `README.md` — keybindings and configuration
- Test: rendering verified by printing `Menu.String()` for each state (Step 2)

**Interfaces:**
- Consumes: `keys.KeyRestart` (Task 4).
- Produces: nothing.

**The trap this task exists to avoid:** see correction 3 above. Appending to the action group
shifts the hardcoded indices and silently breaks both the action-group highlight and the `│`
separators, in a way no test currently catches.

- [ ] **Step 1: Add the entry and compute the boundaries**

Two fields on `Menu`, after `activeTab`:

```go
	// actionGroupStart and actionGroupEnd delimit the action group within options. They
	// are recomputed whenever the options change so that adding an option does not shift
	// the highlighted range or the group separators.
	actionGroupStart, actionGroupEnd int
```

Seed them in `NewMenu` with the previous literals, so states that do not rebuild the options
(`StateEmpty`, `StateNewInstance`, `StatePrompt`) render exactly as before:

```go
		actionGroupStart: 2,
		actionGroupEnd:   5,
```

Set them explicitly in `addInstanceOptions`'s early-return branch for loading instances, which
returns before the group assembly:

```go
		m.actionGroupStart, m.actionGroupEnd = 2, 5
```

Add the entry for instances that are not paused — a paused instance has no running process to
respawn:

```go
	if m.instance.Status == session.Paused {
		actionGroup = append(actionGroup, keys.KeyResume)
	} else {
		// Restarting respawns the pane's process, which a paused instance does not have.
		actionGroup = append(actionGroup, keys.KeyCheckout, keys.KeyRestart)
	}
```

Record the boundaries as the groups are combined:

```go
	// Combine all groups
	m.actionGroupStart = len(options)
	options = append(options, actionGroup...)
	m.actionGroupEnd = len(options)
	options = append(options, systemGroup...)
```

And derive `groups` in `String()` instead of hardcoding it:

```go
		{0, 2},                                 // Instance management group (n, d)
		{m.actionGroupStart, m.actionGroupEnd}, // Action group (enter, submit, checkout/resume, restart)
		{m.actionGroupEnd, len(m.options)},     // System group (tab, help, q)
```

Leave the `inActionGroup` computation reading `groups[1].start`/`groups[1].end` so `groups`
stays the single source of truth.

- [ ] **Step 2: Verify the rendering for every state**

Write a temporary test in package `ui` that prints `Menu.String()` for a running instance, a
paused instance and the empty state, run it, then delete the file.

```go
func TestScratchMenuRender(t *testing.T) {
	for _, st := range []session.Status{session.Running, session.Paused} {
		inst, err := session.NewInstance(session.InstanceOptions{Title: "x", Path: t.TempDir(), Program: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		inst.SetStatus(st)
		m := NewMenu()
		m.SetSize(200, 1)
		m.SetInstance(inst)
		fmt.Printf("status=%v -> %q\n", st, m.String())
	}
	m := NewMenu()
	m.SetSize(200, 1)
	fmt.Printf("empty -> %q\n", m.String())
}
```

Expected, ignoring padding and ANSI codes:

```
running -> n new • D kill │ ↵/o open • p push branch • c checkout • R restart │ tab switch tab • ? help • q quit
paused  -> n new • D kill │ ↵/o open • p push branch • r resume │ tab switch tab • ? help • q quit
empty   -> n new • N new with prompt │ ? help • q quit
```

The paused and empty lines must be identical to the pre-change output — that is the check
that the boundary rework changed nothing it should not have. Delete the scratch test before
committing.

- [ ] **Step 3: Document the keys**

In `app/help.go`'s general help, in the "Managing" block after the `ctrl-q` line. The key
column is padded to width 10:

```go
		keyStyle.Render("R")+descStyle.Render("         - Restart the agent, continuing its conversation"),
		keyStyle.Render("ctrl-x")+descStyle.Render("    - Restart the agent while attached to it"),
```

In `helpTypeInstanceAttach.toContent()`, which already explains `ctrl-q`:

```go
		descStyle.Render("To restart the agent without detaching, press ")+keyStyle.Render("ctrl-x"),
```

In `README.md`, under "Actions" after the `r` line:

```markdown
- `R` - Restart the agent in the selected session, continuing its conversation
- `ctrl-x` - Restart the agent while attached to it, without detaching
```

And under "Configuration", after the sentence about `~/.claude-squad/config.json`:

```markdown
`restart_args` are appended to the program when a session is restarted (`R` / `ctrl-x`) or
when a session is resumed after its tmux session died, so the agent comes back holding its
previous conversation. They are never used on a session's first start, where a fresh worktree
has nothing to continue. The default is `--continue`, which matches Claude Code; set it to
your agent's own resume flag, or to `""` to restart without extra arguments.
```

- [ ] **Step 4: Run the whole suite**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: all packages PASS, vet clean, and no scratch test file left behind
(`git status --short` should show no `ui/menu_render_scratch_test.go`).

- [ ] **Step 5: Commit**

```bash
git add ui/menu.go app/help.go README.md
git commit -m "feat(ui): show R restart in the menu and document the restart keys"
```

---

### Task 7: Acceptance and upstream

**Files:** none.

- [ ] **Step 1: Manual acceptance pass**

Against the installed build from Task 5 Step 3. Every row is a row of the spec's "Поведение"
table.

| Situation | Expected |
|---|---|
| `n new` | Program starts as-is, no `restart_args` |
| `R` in the list | Pane's process restarts with the args; session, branch and worktree untouched |
| `Ctrl+X` while attached | Same, and the user stays attached |
| `R` on a paused instance | Error telling the user to press `r` |
| `R` on an instance whose tmux session died | Error telling the user to press `r` |
| `r resume` after a tmux server death | Rebuilt session comes up with the conversation |
| `restart_args` set to `""` | Restart works, program starts with no additions |
| `Ctrl+X` in the Terminal tab | Forwarded to the shell, no respawn |
| Existing `config.json` without the key | Gains `"restart_args": "--continue"` on first run, rest of the file intact |

The last row is not in the spec; it comes from correction 2 and is the one that decides
whether the feature works for the user who asked for it.

- [ ] **Step 2: Two upstream PRs**

Split as the spec requires — conversation-on-restart, and the restart hotkey — each framed as
a feature. Do not describe either as restoring `--continue`: upstream never had it.

## Self-Review

**Spec coverage.** Every entry in the spec's "Изменения по файлам" maps to a task:
`config/config.go` → 1; `session/tmux/tmux.go` → 2 and 5; `session/instance.go` → 3;
`keys/keys.go` + `app/app.go` → 4; `ui/menu.go` + `app/help.go` → 6. All three "Тестирование"
bullets are covered — tmux command shape in Task 2, config default and file read in Task 1,
`Restart()` guards in Task 3 — and the spec's note that the `Attach()` byte check can only be
checked by hand is carried into Task 5 Step 3. Every row of "Поведение" appears in Task 7
Step 1. "Доставка" is split across the Prerequisites (branch), Task 5 Step 3 (build and
install) and Task 7 Step 2 (PRs). "Вне объёма" is respected: no per-profile args, no
per-instance grouping, and no probing of `~/.claude/projects/` — the `||` fallback delivers
what that probe was meant to protect against without coupling the fork to Claude Code's
storage format.

**Gaps this plan adds beyond the spec:** the missing-key config migration (Task 1) and the
hardcoded menu group boundaries (Task 6). Both are real, both would otherwise ship broken,
and the first would fail silently for every existing user.

**Placeholder scan:** none. Every code step carries the code to write; the two manual steps
name the keys to press and the output to expect; the shell verification in Task 5 Step 2 is a
runnable script with stated expectations and a stop condition.

**Type consistency:** `BuildRestartCommand`, `SetRestartCommand`, `RespawnPane`,
`StartWithRestartCommand`, `restartCommandOrProgram`, `restartCommandFor`, `loadRestartArgs`
and `Instance.Restart` are spelled identically everywhere they appear across Tasks 2-6.
`RespawnPane` is documented in Task 2 as returning `ErrSessionNotFound` and is relied on
under that name in Task 3. The `restartCommand != ""` gate in Task 5 depends on Task 3
setting the command on every agent session, including paused ones — stated in both tasks.

**Ordering:** the branch is created in the Prerequisites, before the first commit — not at the
end. Task 5's gate depends on Task 2 and Task 3; Task 6 depends on Task 4. No task depends on
a later one.
