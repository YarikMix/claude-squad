# Claude Squad [![CI](https://github.com/YarikMix/claude-squad/actions/workflows/build.yml/badge.svg)](https://github.com/YarikMix/claude-squad/actions/workflows/build.yml) [![GitHub Release](https://img.shields.io/github/v/release/YarikMix/claude-squad)](https://github.com/YarikMix/claude-squad/releases/latest)

[Claude Squad](https://smtg-ai.github.io/claude-squad/) is a terminal app that manages multiple [Claude Code](https://github.com/anthropics/claude-code), [Codex](https://github.com/openai/codex), [Gemini](https://github.com/google-gemini/gemini-cli) (and other local agents including [Aider](https://github.com/Aider-AI/aider)) in separate workspaces, allowing you to work on multiple tasks simultaneously.


> **This is a modified fork** of [smtg-ai/claude-squad](https://github.com/smtg-ai/claude-squad),
> maintained at [YarikMix/claude-squad](https://github.com/YarikMix/claude-squad). It adds
> conversation continuity when a session is rebuilt, a restart hotkey, Cyrillic keyboard
> layout support, and several fixes. Releases here are built from this repository and are
> versioned separately from upstream.

![Claude Squad Screenshot](assets/screenshot.png)

### Highlights
- Complete tasks in the background (including yolo / auto-accept mode!)
- Manage instances and tasks in one terminal window
- See added/removed line counts for each task at a glance
- Each task gets its own isolated git workspace, so no conflicts

<br />

https://github.com/user-attachments/assets/aef18253-e58f-4525-9032-f5a3d66c975a

<br />

### Installation

The manual installation below installs this fork as `cs` on your system.

#### Homebrew

Homebrew carries the upstream project, not this fork:

```bash
brew install claude-squad   # installs smtg-ai/claude-squad, without this fork's changes
```

To get this fork, use the manual install below. It puts the binary in `~/.local/bin`, which
most shells search before Homebrew's directory, so it takes precedence if you have both.

#### Manual

Claude Squad can also be installed by running the following command:

```bash
curl -fsSL https://raw.githubusercontent.com/YarikMix/claude-squad/main/install.sh | bash
```

This puts the `cs` binary in `~/.local/bin`.

To use a custom name for the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/YarikMix/claude-squad/main/install.sh | bash -s -- --name <your-binary-name>
```

### Prerequisites

- [tmux](https://github.com/tmux/tmux/wiki/Installing)

### Usage

```
Usage:
  cs [flags]
  cs [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  debug       Print debug information like config paths
  help        Help about any command
  reset       Reset all stored instances
  version     Print the version number of claude-squad

Flags:
  -y, --autoyes          [experimental] If enabled, all instances will automatically accept prompts for claude code & aider
  -h, --help             help for claude-squad
  -p, --program string   Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')
```

Run the application with:

```bash
cs
```
NOTE: The default program is `claude` and we recommend using the latest version.

<br />

<b>Using Claude Squad with other AI assistants:</b>
- For [Codex](https://github.com/openai/codex): Set your API key with `export OPENAI_API_KEY=<your_key>`
- Launch with specific assistants:
   - Codex: `cs -p "codex"`
   - Aider: `cs -p "aider ..."`
   - Gemini: `cs -p "gemini"`
- Make this the default, by modifying the config file (locate with `cs debug`)

<br />

#### Menu
The menu at the bottom of the screen shows available commands: 

##### Instance/Session Management
- `n` - Create a new session
- `N` - Create a new session with a prompt
- `D` - Kill (delete) the selected session
- `↑/j`, `↓/k` - Navigate between sessions

##### Actions
- `↵/o` - Attach to the selected session to reprompt
- `ctrl-q` - Detach from session
- `r` - Resume a paused session
- `R` - Restart the agent in the selected session, continuing its conversation
- `ctrl-x` - Restart the agent while attached to it, without detaching (reserved by claude-squad, so an agent CLI that binds `ctrl-x` itself won't see it)
- `?` - Show help menu

##### Navigation
- `tab` - Switch between preview and terminal tabs
- `q` - Quit the application
- `shift-↓/↑` - scroll in preview/terminal view

### Configuration

Claude Squad stores its configuration in `~/.claude-squad/config.json`. You can find the exact path by running `cs debug`.

`restart_args` are appended to the program when a session is restarted (`R` / `ctrl-x`) or
when a session is resumed after its tmux session died, so the agent comes back holding its
previous conversation. They are never used on a session's first start, where a fresh worktree
has nothing to continue. The default is `--continue`, which matches Claude Code; set it to
your agent's own resume flag, or to `""` to restart without extra arguments. If your program
contains shell operators (`;`, `&&`, `||`, `|`, ...), there is no unambiguous place to append
the args, so a restart runs the program unchanged and will not continue the conversation. Note
also that a restart kills the running agent process outright (`respawn-pane -k` sends
SIGKILL), so anything it had in flight is lost.

#### Profiles

Profiles let you define multiple named program configurations and switch between them when creating a new session. When more than one profile is defined, the session creation overlay shows a profile picker that you can navigate with `←`/`→`.

To configure profiles, add a `profiles` array to your config file and set `default_program` to the name of the profile to select by default:

```json
{
  "default_program": "claude",
  "profiles": [
    { "name": "claude", "program": "claude" },
    { "name": "codex", "program": "codex" },
    { "name": "aider", "program": "aider --model ollama_chat/gemma3:1b" }
  ]
}
```

Each profile has two fields:

| Field     | Description                                              |
|-----------|----------------------------------------------------------|
| `name`    | Display name shown in the profile picker                 |
| `program` | Shell command used to launch the agent for that profile  |

If no profiles are defined, Claude Squad uses `default_program` directly as the launch command (the default is `claude`).

### FAQs

#### Failed to start new session

If you get an error like `failed to start new session: timed out waiting for tmux session`, update the
underlying program (ex. `claude`) to the latest version.

### How It Works

1. **tmux** to create isolated terminal sessions for each agent
2. **git worktrees** to isolate codebases so each session works on its own branch
3. A simple TUI interface for easy navigation and management

### License

[AGPL-3.0](LICENSE.md)

### Star History

[![Star History Chart](https://api.star-history.com/svg?repos=smtg-ai/claude-squad&type=Date)](https://www.star-history.com/#smtg-ai/claude-squad&Date)
