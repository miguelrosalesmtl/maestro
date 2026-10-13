# Maestro

Your Jira tickets, conducted. A desktop app (Go + [Wails](https://wails.io)) that keeps the
ticket and the code in one place: start work on a ticket, and its branch, commits, PR and
merge move the Jira status for you — with a real terminal bound to each ticket.

> **Status:** live Jira sync over the REST API and GitHub pull request tracking. Without a
> Jira login the app shows demo data. Not yet tried against a real Jira site: expect rough edges,
> and send the log file (see [Logs and crashes](#logs-and-crashes)) when something fails.

## Install and try it

You need: a Jira site where you can create an API token, and (for pull requests) a GitHub
account with access to your repositories. The one-time Jira login uses the `maestro-jira`
command-line tool, which is not in the download yet, so Go is needed for that step.

### macOS

**1. Get the app.** Every push builds a universal app (Apple Silicon and Intel).

- In the browser: [Actions](https://github.com/miguelrosalesmtl/maestro/actions) → the latest
  green **build** run → *Artifacts* → **Maestro-macos-universal**, or
- with the GitHub CLI: `gh run download -R miguelrosalesmtl/maestro -n Maestro-macos-universal`
  (latest run).

```sh
unzip Maestro-macos-universal.zip            # skip if the browser already unzipped it
mv Maestro.app /Applications/
xattr -dr com.apple.quarantine /Applications/Maestro.app
```

The app is not signed yet. Without the `xattr` line, macOS says it "can't be opened"; you can
also right-click the app → **Open** → **Open** once. Downloads from Actions expire after 90 days.

**2. Install the tools and get the code** (for the login tool):

```sh
brew install go gh
gh auth login                                # lets Maestro read your pull requests
git clone https://github.com/miguelrosalesmtl/maestro.git
cd maestro
```

**3. Connect Jira** (once):

```sh
go run ./cmd/maestro-jira login              # asks for your site, email and API token
go run ./cmd/maestro-jira discover -apply    # reads your projects, statuses and fields into the config
```

For Jira Cloud, create the token at
<https://id.atlassian.com/manage-profile/security/api-tokens>; for Data Center, use a personal
access token (Jira → Profile → Personal Access Tokens). The token is stored in the macOS
Keychain, never in a file. The config is written to
`~/Library/Application Support/maestro/config.yaml`.

**4. Add your repositories** to that file, under each project, so Maestro follows their pull
requests and opens ticket terminals in the right folder:

```yaml
projects:
  - key: ABC
    repos:
      - name: your-org/your-repo             # owner/name on GitHub
        path: ~/code/your-repo               # your local clone
```

Then check everything:

```sh
go run ./cmd/maestro-jira check              # Jira token, permissions, config vs. Jira, GitHub, each repo
```

**5. Open Maestro** from Applications. The sidebar shows **Jira · your-site** and
**GitHub · @you** (or says why one is not connected). Logs are in `~/Library/Logs/Maestro/`.

To update later: download the newest build, replace `/Applications/Maestro.app`, and run the
`xattr` line again. Your config and Keychain token stay.

### Linux

Download **Maestro-linux-amd64** from the same Actions run (or build it: see
[Develop](#develop)). It needs GTK 3 and WebKitGTK 4.1:

```sh
sudo pacman -S gtk3 webkit2gtk-4.1                     # Arch / Omarchy
sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0        # Debian / Ubuntu
tar -xzf Maestro-linux-amd64.tar.gz && ./maestro
```

Steps 2–4 are the same as on macOS (`go` and `gh` from your package manager). The config is in
`~/.config/maestro/config.yaml`, the token in the Secret Service keyring (GNOME Keyring,
KWallet…), and logs in `~/.local/state/maestro/logs/`.

## Views

The ticket tree on the left is resizable: drag its right edge (200–720 px), double-click the
edge to reset. Next to its filter, a sprint selector shows one sprint at a time: *Current*
(any active sprint), a specific sprint, or *Backlog*; epics and initiatives stay as context.
Both are remembered.

| | |
|---|---|
| **My work** | Everything assigned to you, grouped by status, with sub-task progress, blockers and dev flow. Switch to *Everyone* or to a teammate to browse tickets that aren't yours. The bar shows only you and the people you pin; **Find person** (`w`) searches the whole org: Enter views someone, the pin (or Ctrl+Enter) keeps them in the bar. `]` / `[` cycle the bar. The **All / current sprint** switch (`s`) keeps only tickets in an active sprint, on any of your boards (with several teams, several sprints are active at once); the choice is remembered. |
| **Focus** | The ticket you're on: Ticket ⇄ Code bridge (Jira statuses above, dev stages below, automation links between them), code/PR panel, sub-tasks, linked issues, timeline, up-next queue. |
| **Terminal dock** | Real shell (PTY) per ticket. `$MAESTRO_TICKET`, `$MAESTRO_BRANCH`, `$MAESTRO_SUMMARY` are set; one-click ticket commands; `git checkout -b`, `git push`, `gh pr create`, `gh pr merge` typed here advance the flow and the Jira status. |
| **Board** | *Your sprint* (the active sprint of your projects), or any Jira board: **Find board** (`w`) searches every board on the site; pin the ones you follow. A board shows its own Jira columns and any sprint you pick (active, upcoming, recently closed); kanban boards show open work. Drag a card to transition it; `r` reloads, `]` / `[` switch boards. Pins and the last sprint per board are remembered. |
| **Roadmap** | Initiative → Epic → tickets with rolled-up progress. |
| **Automations** | Rules: *when* dev event *then* move ticket to status. |
| **Freestyle** | Terminals not linked to a ticket, for investigations. They start in your home folder, get no ticket variables, and never move a ticket. Double-click a tab to rename it; tabs are remembered. |
| **Jira view** | Any ticket rendered like Jira: description, child issues / sub-tasks, linked issues (add/remove), comments, history. |
| **Create** | Initiative → Epic → Story/Task/Bug → Sub-task, validated against the hierarchy. From the `+` on any tree row, the Jira view, the palette, or `C`. |

## Keyboard

| Keys (Linux; ⌘ on macOS) | |
|---|---|
| `Ctrl+1` … `Ctrl+6` | My work, Focus, Board, Roadmap, Automations, Freestyle (works even in the terminal) |
| `F11` | Full screen (or the button in the top bar) |
| `Ctrl+Shift+M` / double-click the top bar | Maximize / restore |
| `?` | Every shortcut, rebindable |

On tiling window managers (e.g. omarchy/Hyprland) the window manager decides the window size;
use `F11` here or your WM's fullscreen key (omarchy: `Super+F`).

Press **`?`** for every shortcut in the current view; click one to rebind it (two-key
sequences like `g b` work). Overrides are saved to `<config dir>/maestro/keybindings.json`
(`~/.config/maestro` on Linux, `~/Library/Application Support/maestro` on macOS) and can be
edited by hand. **`f`** shows Vimium-style link hints for anything clickable; **`F`** shows hints on
tickets only; picking one is the same as clicking it (the Jira view is then `J` inside the ticket).

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

What is live: issues in your projects and anything assigned to you in any project (open work +
anything updated in the last 14 days, plus their epics/initiatives), polled every 60 s (15 s
right after you change something). **Refresh now** with the ↻ button next to the Jira status
in the sidebar, or `Shift+R`;
status changes use real Jira transitions (a required Resolution is filled in); create,
comment, link/unlink; changes made in Jira by others appear on the ticket timeline.
"Start work" types `git checkout -b …` into the ticket's terminal (opened in the project's
repository, `projects[].repos[].path`). From there, what happens on GitHub (PR opened, review,
merge) moves the ticket through your automation rules: see
[GitHub pull requests](#github-pull-requests).

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
ticket's env vars and repo folder, and git/gh commands in any of them drive the ticket. The panel
shows only the selected ticket's terminals; the others keep running behind the "N others" button.
Selecting a ticket never starts a shell. In the Freestyle view the same keys act on its terminals.

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

### GitHub pull requests

With Jira connected, Maestro follows the pull requests of every repository in
`projects[].repos` and moves tickets from what really happens on GitHub:

| On GitHub | Default rule |
|---|---|
| A branch with the key appears (local clone or PR) | → In Progress |
| PR opened, or a draft marked ready | → In Review |
| Changes requested | → In Progress |
| PR merged | → Done |
| A required check fails | → Blocked (off by default) |

- **Matching:** the issue key (`dev.keyPattern`) in the head branch, else the PR title, else the
  body. Only keys of issues Maestro has synced count.
- **Checks and reviews:** check runs and commit statuses on the head commit; `dev.checks.required`
  picks the ones that decide (empty = all). Approvals count each reviewer's latest review, as
  GitHub does; `dev.review.requiredApprovals` sets how many are needed.
- **Local work:** branches in the repo's `path` that name a ticket show up with their commits
  (and line counts) before a PR exists.
- **Startup is quiet:** the first look only shows the current state. Only changes seen while
  Maestro runs fire rules, so an old merged PR never moves a reopened ticket to Done.
- **Jira link:** each PR is added to the issue as a web link (resolved once merged or closed).
  Turn off with `jira.writeBack.remoteLinks: false`.
- **Cost:** one PR listing per repo per `dev.github.pollSeconds` (default 60), with ETags, so
  an unchanged repo costs no rate limit. Typing `git`/`gh` in a ticket terminal checks again
  within seconds, then every 15 s for two minutes.

Sign-in uses the GitHub CLI by default (`gh auth login`; the token is read from `gh` on every
call, never stored by Maestro). Without `gh`, store a fine-grained token (read access to pull
requests, checks, commit statuses and contents) with `go run ./cmd/maestro-jira github-login`
and set `dev.github.auth: token`. `maestro-jira check` tests the sign-in and each repository.
The sidebar shows `GitHub · @you`, or why it is not connected.

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

Download them from the run's *Artifacts* (see [Install and try it](#install-and-try-it)).
Pushing a `v*` tag also creates a GitHub Release.

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
cmd/maestro-jira         CLI: login, check, discover, detect, logout, github-login
internal/flow            Dev flow per ticket, automation rules, terminal command reactions, ApplyDev
internal/github          GitHub REST client: gh/keychain auth, ETags, retries, rate limits
internal/github/ghtest   Fake GitHub server for tests
internal/devsync         Pull request + local git watcher → flow.DevState
internal/term            PTY shell sessions per ticket
frontend/src             React + TypeScript UI
```
