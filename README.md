> Local development branch: safe manual session transfer MVP. See [Cursor terminal guide](docs/SESSION-TRANSFER.ko.md). No new automatic limit-based switching.

# ccam — Claude Code Account Manager

**English** · [한국어](README.ko.md) · Website: https://ccam.dev

- ccam is a free, open-source command-line tool that lets you use several Claude Code accounts on one computer and switch between them with one command.
- Each account keeps its own login in a separate CLAUDE_CONFIG_DIR, while MCP servers, settings, CLAUDE.md, extensions and conversation history can be shared between accounts, so you can continue a conversation on another account when one reaches its usage limit.
- Install it on Linux, macOS or Windows with one command, then run ccam add for each account.
- ccam is an unofficial project and is not affiliated with or endorsed by Anthropic.

`ccam` is a single Go binary for adding and switching between several Claude Code accounts in a terminal on Linux, macOS or Windows. Each account gets its own `CLAUDE_CONFIG_DIR`, so logins stay separate, and you choose per item whether MCP servers, settings, the global CLAUDE.md, extensions and conversation history are shared.

> **Check the terms first.** Whether using several accounts is allowed depends on each account's Anthropic terms (the [Consumer Terms](https://www.anthropic.com/legal/consumer-terms) for Pro and Max, the [Commercial Terms](https://www.anthropic.com/legal/commercial-terms) for Team and Enterprise, the [Usage Policy](https://www.anthropic.com/legal/aup) for all) and your organization's policy. ccam does not raise, merge or hide an account's limits: every account still stops at its own. ccam carries your work on as another of your accounts when you ask, or by itself at a limit when you turn on `auto_switch` (Linux and macOS). Whether that is allowed for your accounts is your call.

> **Team and Enterprise seats.** Automatic choice (`ccam next`, `--fallback`, `ccam pick`, the dashboard's `n`, and [`auto_switch`](https://ccam.dev/en/features/next/#auto-switch) when you turn it on) also switches between seats of the same Claude Team or Enterprise organization. Anthropic's help center says "Each seat on a Team plan is assigned to one named individual, and only that person may use it" and "Users can't route their work through another user's account to use that user's remaining usage" ([Team plans](https://support.claude.com/en/articles/12004354-purchase-and-manage-seats-on-team-plans#h_b8a7d07129); likewise for [Enterprise plans](https://support.claude.com/en/articles/13393991-purchase-and-manage-seats-on-enterprise-plans#h_fa6b9cade0)). Using automatic choice between seats may therefore violate Anthropic's terms. Whether you use it there is your call: check with your organization and Anthropic before you rely on it. Personal plans (Pro, Max) are covered by the note above.

All program output (messages, prompts, help) is in English.

## Why ccam

- **Logins are never copied.** Each account signs in once in its own folder, and Claude Code renews its own token there. Switching never logs an account out, and several accounts can run at once.
- **Nothing runs beside Claude Code by default.** ccam `exec`s into `claude`: no wrapper, proxy or daemon, and about 4 ms added at launch. (Windows has no `exec`; there ccam waits for Claude Code and passes its exit code on.) With `relay = true` (or `auto_switch = true`, on Linux and macOS) in `config.toml`, ccam waits beside each interactive session instead, so it can carry the conversation on as another account.
- **Groups decide what is shared.** A group names the items it shares (MCP servers, settings, CLAUDE.md, extensions, history) and its accounts; an account can be in several. MCP servers go through `--mcp-config`, so a launch leaves `.claude.json` alone (ccam writes it, with a backup, only when an account leaves or rejoins MCP sharing, on `ccam mcp import`, and for Remote Control).
- **Changes are planned and backed up.** A plan before any files move (naming the files inside folders that differ), backups, `--dry-run`, atomic writes, and `ccam doctor --fix`, which also finishes a change that stopped halfway.
- **Limits handled for scripts too.** `ccam --fallback work -p "…"` runs the same command again as the account with the most room when `work` reaches its limit, and `ccam pick` gives each parallel worker its own account; both exit with 75 (or `--wait`) when nobody has room. Interactive sessions get a `ccam next --auto --continue` hint in the status line, and with `auto_switch = true` (Linux, macOS; off by default, experimental) ccam moves a session that Claude Code stops at its limit to another account with room by itself. Within one Team or Enterprise organization these choices, `auto_switch` included, also move work between seats, which may violate Anthropic's terms (see "Team and Enterprise seats" above).
- **The right account per project.** `ccam folder work ~/code/company` makes name-less launches there use `work`.
- **One static binary.** Linux, macOS and Windows, no runtime to install, checksum-verified install and update, no telemetry.

How this compares with claude-swap, CCS, clauth, CAAM, cux and others: [ccam.dev/en/compare/account-switchers](https://ccam.dev/en/compare/account-switchers/).

## Install

```bash
curl -fsSL https://ccam.dev/install.sh | sh
```

Linux (Ubuntu, Debian, Fedora, Arch, Alpine, …) and macOS are supported. This installs `~/.local/bin/ccam` after verifying the release checksum.

On Windows 10 or 11, in PowerShell:

```powershell
irm https://ccam.dev/install.ps1 | iex
```

This installs `%USERPROFILE%\.local\bin\ccam.exe` after verifying the checksum and adds the folder to your user `PATH`. Shared folders work as they are; sharing single files (`settings.json`, `CLAUDE.md`, `history.jsonl`) needs Developer Mode, because Windows lets only that mode (or an administrator) link files, so without it the first `ccam add` creates group `default` with MCP servers and extensions only. Running the script again updates ccam, also while a session runs from the old binary, and renews shortcuts that are hard links. Alternatives: the `.deb` from [Releases](https://github.com/jhyunwoo/ccam/releases), or `go install github.com/jhyunwoo/ccam/cmd/ccam@latest` (Go 1.26+; a build of a release tag reports its version, so `ccam update` works for it). Claude Code (`claude`) must be installed.

Claude Code plugin (Claude Code 2.1.275+): `/plugin install ccam --marketplace jhyunwoo/ccam` teaches Claude Code to check usage, start workers on other accounts with `ccam pick`, and explain missing skills with `ccam ext`.

## Update

```bash
ccam update            # install the latest release
ccam update --check    # only say whether a newer release exists
```

`ccam update` downloads the release for your system, checks it against the release's `checksums.txt` (SHA-256) and replaces the `ccam` binary in one rename; shortcut commands link to it, so they update too. `--version v0.2.0` installs a specific release (also an older one), and `ccam --dry-run update` shows what would be downloaded and replaced. The dashboard (`ccam` on a terminal) asks GitHub for the latest release each time it opens and, when there is a newer one, offers it: press `U` and confirm. No other command checks, and ccam never updates without asking. A `.deb` install is left to apt: download the new `.deb` from [Releases](https://github.com/jhyunwoo/ccam/releases) and run `sudo apt install ./ccam_<version>_linux_<arch>.deb`. Rerunning the install script also updates a `~/.local/bin` install.

## Quick start

```bash
ccam add             # name → sign-in → groups → shortcut? → summary → browser login
ccam add work --shortcut ccwork    # second account with the shortcut command ccwork

ccwork               # Claude Code as account work (via its shortcut)
ccam work --resume   # by name; everything after the name goes to claude unchanged
ccam                 # dashboard: accounts, agents, groups, folders, extensions, health
ccam folder work ~/code/company   # name-less launches in that folder use work
ccam usage           # 5-hour, weekly and per-model (Fable) usage of every account
ccam next --continue # out of quota? continue the same conversation on the next account
ccam --fallback work -p "fix the tests"   # in scripts: rerun on another account at the limit
```

An existing `~/.claude` can be registered in place with `ccam import` (nothing is moved or copied).

To start the shared settings from the setup you already have, `ccam migrate` copies `~/.claude`'s settings.json, CLAUDE.md, extensions and MCP servers into the shared group. The original folder is only read; conversation history is copied only with `--history`. On a terminal it first shows a checklist of the items to copy, then prints the plan and asks (`ccam --dry-run migrate` only prints it). The saved sign-ins (OAuth) of MCP servers are not copied, since a copied sign-in logs the other holders out; the plan names the accounts that sign in with `/mcp`.

Signing in on a remote machine over SSH: the browser on your computer ends on a `http://localhost:<port>/callback?code=…` page that does not load. Paste that address into the terminal where `ccam add` or `ccam login` runs, and ccam passes it to Claude Code on the server.

## Commands

| Command | What it does |
| --- | --- |
| `ccam` | Full-screen dashboard (prints the list when not on a terminal) |
| `ccam <name\|number> [claude args]` | Run `claude` as that account; args are passed through verbatim |
| `<shortcut> [claude args]` | Launch through the account's shortcut command, if it has one |
| `ccam add [name]` | Add an account (wizard on a terminal; `--no-login`, `--group <group>`, `--no-group`, `--shortcut <cmd>`; `--api --key-from env:NAME\|command:CMD` for an API key account) |
| `ccam shortcut <name> [cmd] [--remove]` | Show, create, rename or remove an account's shortcut command |
| `ccam list [--json]` | List accounts |
| `ccam use <name>` | Default account for name-less runs such as `ccam -p "…"` |
| `ccam folder [name] [folder] [--remove]` | Account for name-less runs in a folder and below; without arguments, list them |
| `ccam --fallback [name] -p …` | Run again as the account with the most room when the account reaches its usage limit (`-p` runs only) |
| `ccam next [--auto] [--continue [--brief]] [-- claude args]` | Next account; `--auto` picks by usage (`--max-usage`, `--strategy headroom\|expiring\|balance`, `--wait`) and says why; `--continue --brief` starts a new conversation from a summary of the last one. Claude's own options go after `--`: `ccam next -c --auto -- --dangerously-skip-permissions` |
| `ccam pick [--json] [--model m] [--wait]` | Print the account with the most room for a script; workers started together get different accounts; exit 75 when none |
| `ccam ext [name] [--json]` | Skills, commands, plugins, MCP servers and hooks each account loads, with their source and problems |
| `ccam usage [--watch] [--json] [-v]` | Usage (`ccam u`) |
| `ccam ccusage [-a name[,name…]] [ccusage args]` | [ccusage](https://ccusage.com)'s token and cost reports over the conversations of every account, or of the accounts `-a` names (one that shares its history through a group brings the group's conversations). ccusage's own arguments go on unchanged, and it also counts the other coding tools it finds unless you name one: `ccam ccusage claude daily`. `blocks` needs one account, as each has its own 5-hour windows: `ccam ccusage -a work blocks --active`. Runs an installed ccusage, else `bun x ccusage`, else `npx ccusage@latest` |
| `ccam group [add\|set\|join\|leave\|show\|rename\|remove]` | Groups: what a set of accounts share; changes print a plan and ask first |
| `ccam share <name>` | What an account shares, and through which group |
| `ccam mcp add\|add-json\|list\|remove\|import` | Shared MCP servers |
| `ccam login\|logout <name>` | Log in again / log out (the login and its MCP sign-ins; asks first when something goes with it) |
| `ccam rename` / `ccam remove` | Rename / remove (logged out first, then the folder goes to a trash folder) |
| `ccam import [dir]` | Register an existing config folder |
| `ccam migrate [dir]` | Copy an existing setup (default `~/.claude`) into the shared group; `--group`, item flags, `--history`, `--yes` |
| `ccam doctor [--fix]` | Check links, permissions, login, version, usage format; repair |
| `ccam statusline install` | Show `[account] · model · 5h N%` in Claude Code's status line |
| `ccam update [--check] [--version vX.Y.Z]` | Update ccam to the latest (or that) release, checksum verified |
| `ccam uninstall` | Remove every account (logged out first), the shortcuts, ccam's folders and ccam itself; an imported folder stays |
| `ccam completion bash\|zsh\|powershell` | Shell completion |

Put `--dry-run` first to print what would happen (`ccam --dry-run work`). It works for launching (`ccam <name>`, `run`, `next`), `group`, `migrate`, `shortcut`, `update`, `uninstall` and `doctor --fix`; other commands that change something refuse it rather than run for real.

## Sharing

A group shares the items it names between the accounts in it: `ccam group add team --items history --members work,side`. An account can be in several groups, but each item comes from one group only; the first account creates `default` (MCP servers, settings, CLAUDE.md, extensions) and later ones join it. MCP servers are passed with `--mcp-config <group>/mcp.json` at launch (`.claude.json` untouched at launch; `mcp_strategy = "sync"` merges into it instead, keeping the account's own servers). An account that leaves MCP sharing keeps the group's servers in its `.claude.json`, unless `--fresh`. Settings, CLAUDE.md, extension folders and `projects/` + `history.jsonl` are symlinked to the group folder. `.credentials.json` and `.claude.json` are never shared. A group change moves every account it concerns in one plan, backs originals up to `.ccam-backup/<time>/` and lets you resolve differing files (keep group / use mine / merge JSON). A `config.toml` from before groups is read as groups and rewritten at the next save, with the old file kept as `config.toml.bak`.

## Usage lookup

Queries every account's usage in parallel with its own token (3 s per account, 60 s cache). The endpoint is undocumented and may change; on failure the last known or status-line values are shown and `ccam doctor` reports format changes. A window Claude Code reported full (through the plugin's hook) counts as 100% until its reset. Expired tokens are not refreshed unless you pass `--refresh` (a renewal may take up to 30 s), and never while that account is running, however Claude Code was started.

## Performance

`ccam <name>` reads one file, checks links with `lstat`, and `exec`s into `claude`: about 4 ms p95 added on a typical laptop, 8 MB peak memory. On Windows, which has no `exec`, ccam starts `claude.exe` with the same console and waits for it; with `relay = true` or `auto_switch = true`, ccam starts interactive sessions as a child and waits beside them on Linux and macOS too. `CCAM_TRACE=1` prints per-step timings; `CCAM_PPROF=<file>` writes a CPU profile.

## Development

```bash
go test ./...
golangci-lint run
scripts/perf-check.sh           # launch-path targets with hyperfine
scripts/bench-compare.sh main   # fail on >20% benchmark regressions (benchstat)
```

Design and decision log: [docs/PLAN.md](docs/PLAN.md). Claude Code folder findings: [docs/claude-layout.md](docs/claude-layout.md).

## License

[MIT](LICENSE). ccam is an unofficial project and is not affiliated with or endorsed by Anthropic.
