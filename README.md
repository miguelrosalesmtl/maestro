# Maestro

Your Jira tickets, conducted. A desktop app (Go + [Wails](https://wails.io)) that keeps the
ticket and the code in one place: start work on a ticket, and its branch, commits, PR and
merge move the Jira status for you — with a real terminal bound to each ticket.

> **Status:** live Jira sync over the REST API (log in once with `maestro-jira login`); without a login the app shows demo data. GitHub integration comes next.

## Views

| | |
|---|---|
| **My work** | Everything assigned to you, grouped by status, with sub-task progress, blockers and dev flow. Switch to any teammate or *Everyone* to browse tickets that aren't yours. |
| **Focus** | The ticket you're on: Ticket ⇄ Code bridge (Jira statuses above, dev stages below, automation links between them), code/PR panel, sub-tasks, linked issues, timeline, up-next queue. |
| **Terminal dock** | Real shell (PTY) per ticket. `$MAESTRO_TICKET`, `$MAESTRO_BRANCH`, `$MAESTRO_SUMMARY` are set; one-click ticket commands; `git checkout -b`, `git push`, `gh pr create`, `gh pr merge` typed here advance the flow and the Jira status. |
| **Board** | Sprint board; drag a card to transition it. |
| **Roadmap** | Initiative → Epic → tickets with rolled-up progress. |
| **Automations** | Rules: *when* dev event *then* move ticket to status. |
| **Jira view** | Any ticket rendered like Jira: description, child issues / sub-tasks, linked issues (add/remove), comments, history. |
| **Create** | Initiative → Epic → Story/Task/Bug → Sub-task, validated against the hierarchy. From the `+` on any tree row, the Jira view, the palette, or `C`. |

## Keyboard

| Keys (Linux; ⌘ on macOS) | |
|---|---|
| `Ctrl+1` … `Ctrl+5` | My work, Focus, Board, Roadmap, Automations (works even in the terminal) |
| `F11` | Full screen (or the button in the top bar) |
| `Ctrl+Shift+M` / double-click the top bar | Maximize / restore |
| `?` | Every shortcut, rebindable |

On tiling window managers (e.g. omarchy/Hyprland) the window manager decides the window size;
use `F11` here or your WM's fullscreen key (omarchy: `Super+F`).

Press **`?`** for every shortcut in the current view; click one to rebind it (two-key
sequences like `g b` work). Overrides are saved to `<config dir>/maestro/keybindings.json`
(`~/.config/maestro` on Linux, `~/Library/Application Support/maestro` on macOS) and can be
edited by hand. **`f`** shows Vimium-style link hints for anything clickable; **`F`** picks a
ticket to open in the Jira view.

## Jira configuration

[`docs/jira-configuration-spec.md`](docs/jira-configuration-spec.md) lists every piece of Jira (and
local) configuration Maestro needs, how it is discovered, and the draft YAML keys. Written in
ASD-STE100 Simplified Technical English.

### Connecting to Jira

```sh
go run ./cmd/maestro-jira login              # site + API token → keychain, writes config.yaml
go run ./cmd/maestro-jira check              # validates config, tests the token and permissions
go run ./cmd/maestro-jira discover           # shows what Maestro would configure
go run ./cmd/maestro-jira discover -apply    # …and saves it
```

Then restart Maestro: the sidebar shows a green **Jira · your-site** dot and your real issues.
Without a login (or if the connection fails) it shows sample data and says why.

What is live: issues in your projects (open work + anything updated in the last 14 days,
plus their epics/initiatives), polled every 60 s (15 s right after you change something);
status changes use real Jira transitions (a required Resolution is filled in); create,
comment, link/unlink; changes made in Jira by others appear on the ticket timeline.
"Start work" types `git checkout -b …` into the ticket's terminal (opened in the project's
repository, `projects[].repos[].path`); `git push`, `gh pr create`, `gh pr merge` there move
the ticket through your automation rules.

The token goes to the OS keychain only; `config.yaml` holds a `credentialRef`. See
[`docs/config.example.yaml`](docs/config.example.yaml) for every key (a test keeps it valid).

### Fields, sprints and versions

- **System fields** are read and shown: fix/affects versions, due date (overdue in red),
  resolution, environment, time tracking, security level (🔒), attachments, watchers, creator,
  time in status. Fix/affects versions, due date and custom fields are editable in the Jira view
  (only what Jira's edit screen allows).
- **Atlassian fields** (Sprint, story points, Flagged, Start date, Team, Epic Link…) are found
  by schema type, never by a fixed `customfield_` ID. Story points follow the board's estimation
  field (`fields.storyPoints: auto`). Flagged issues count as blocked.
- **Your custom fields:** list them under `fields.custom` (name or ID, where to show them,
  a role such as `severity` or `acceptanceCriteria`, defaults, and team rules like
  `requiredOn: [inReview]`). Any field type is decoded; most can be edited and set on create.
  Without that list, the Jira view shows every custom field that has a value.
- **Sprints:** parallel active sprints (picker on the board), goal and days left, future sprints
  in the create dialog, a "carried over ×N" badge. **Versions:** unreleased + recently released.
- **Secured issues:** their summary never goes into a branch name, PR title or the terminal.
- `maestro-jira check` compares your config with Jira (missing fields, versions, story points).

Not yet: managing sprints/versions, velocity and sprint progress, release notes, log work.

### Ticket terminals

Each ticket can have several terminals (`PLAT-142`, `PLAT-142 #2`, …); all of them carry the
ticket's env vars and repo folder, and git/gh commands in any of them drive the ticket.

| Keys (Linux; ⌘ on macOS) | |
|---|---|
| `` Ctrl+` `` | Open the terminal / jump into it / back to the app |
| `t` | Focus the current ticket's terminal |
| `Ctrl+Shift+T` | New terminal for this ticket |
| `Ctrl+PageDown` / `Ctrl+PageUp` | Next / previous terminal |
| `Ctrl+Shift+W` | Close this terminal |
| `Ctrl+Shift+J` | Show / hide the terminal panel |

**Activity light:** a ticket with a terminal shows a terminal icon everywhere (sidebar, My work,
board card, Focus header, tab). Cyan spinner = a command is running, amber pulse = a command
has gone quiet (probably waiting for input, e.g. an AI agent's question), green = a long command
finished. You get a short chirp and a notice when a command finishes after ≥ 10 s or starts
waiting — unless you are looking at that terminal. Interactive programs (vim, less, ssh, …)
never notify. Settings: `terminal.notify` in `config.yaml`; mute from the command palette.

## Logs and crashes

| | macOS | Linux |
|---|---|---|
| Folder | `~/Library/Logs/Maestro` | `~/.local/state/maestro/logs` (`$XDG_STATE_HOME`) |

- `maestro.log`: JSON lines (`log/slog`), rotated at 10 MB, 5 gzipped backups, 30 days.
  Backend errors, failed Jira operations, automations, terminal sessions, Wails messages
  and UI errors (uncaught exceptions, rejected promises, React render errors) all land here.
- `crash-<time>.log`: the Go runtime's report if the app crashed hard. The next start
  logs "previous run crashed" and shows a notice in the app.
- Level: `log.level` in `config.yaml`, or `MAESTRO_LOG_LEVEL=debug` (wins).
- Privacy: only keys, IDs, statuses and errors are logged, never ticket text, terminal input
  or tokens; sensitive attribute names are redacted as a safety net.
- In the app: command palette → **Open log folder**, or click an error notification.

```sh
tail -f ~/.local/state/maestro/logs/maestro.log | jq .                  # follow
jq 'select(.level=="ERROR")' ~/.local/state/maestro/logs/maestro.log    # errors only
```

## Develop

Requires Go 1.25+, Node 20+, and the Wails CLI:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
make dev     # hot reload; also at http://localhost:34115
make test
make build   # → build/bin/
```

Linux needs `webkit2gtk-4.1` (the Makefile adds `-tags webkit2_41` automatically).

## Build from GitHub

`.github/workflows/build.yml` builds on every push/PR:

- **macOS** universal (`Maestro.app`, zipped) on `macos-latest`
- **Linux** amd64 binary

Download them from the run's *Artifacts*. Pushing a `v*` tag also creates a GitHub Release.
The macOS app is unsigned for now; after unzipping run
`xattr -dr com.apple.quarantine Maestro.app` (or right-click → Open).

## Layout

```
main.go, app.go          Wails entry + methods bound to the UI
internal/tracker         Tracker interface + in-memory mock used by the UI today
internal/tracker/live    Live Jira tracker: mapping, polling sync, writes, link-direction check
internal/tracker/jira/jiratest  Fake Jira server for tests (also over HTTPS)
internal/tracker/jira    Jira REST client: auth, retries, rate limits, pagination, operations, ADF
internal/config          config.yaml + team file: schema, merge, validation, import, keybindings
internal/secrets         OS keychain credential refs (keychain:… / env:…), redacting Secret type
internal/applog          Log file (slog JSON, rotation), crash reports, panic recovery, UI errors
internal/setup           Discovery → proposals (roles, states, fields, boards), Connect(config) → client
cmd/maestro-jira         CLI: login, check, discover, detect, logout
internal/flow            Dev flow per ticket, automation rules, terminal command reactions
internal/term            PTY shell sessions per ticket
frontend/src             React + TypeScript UI
```
