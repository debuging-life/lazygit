# deskgit

**deskgit** is DeskTimers' terminal git client. It connects your local git workflow to your [DeskTimers](https://desktimers.com) workspace: pick the task you're working on, and every branch, commit, and push you make is automatically tagged with its task code and mapped back to the task in DeskTimers — no matter which editor or git client you commit from.

deskgit is a customized fork of the excellent [lazygit](https://github.com/jesseduffield/lazygit) by Jesse Duffield and contributors (MIT). All of lazygit's features work exactly as you know them — deskgit adds a thin DeskTimers layer on top, and installs fully independently of any existing lazygit (own binary name, own `~/.config/deskgit/` config).

## How it works

```
deskgit (TUI)          →  pick a task, branches prefill with its code
dt-hook (git hooks)    →  prepare-commit-msg prefixes commits, pre-push warns on untagged ones
DeskTimers webhooks    →  the server maps commits/branches/PRs to tasks by code
```

Task codes are your workspace's existing Jira-style codes: `<PROJECT>-<number>`, e.g. `MOB-101`. Because tagging happens in **git hooks**, commits made from VS Code, IntelliJ, or plain `git` on a hooked repo get tagged too — deskgit is just the comfortable way to drive it.

## Install

### Homebrew (macOS / Linux)

```sh
brew install debuging-life/tap/deskgit
```

This installs both `deskgit` and its hook companion `dt-hook` from the latest tagged release. Use `brew install --HEAD debuging-life/tap/deskgit` to build the latest `main` branch instead.

### From source

Requires Go 1.25+:

```sh
git clone git@github.com:debuging-life/lazygit.git deskgit && cd deskgit
git checkout main
sh scripts/build-deskgit.sh ~/.local/bin   # installs `deskgit` and `dt-hook`
```

## First run

1. Run `deskgit` inside any git repo.
2. It prints a code like `BDBG-GSGX` and opens `https://leads.desktimers.com/device` in your browser (RFC 8628 device flow, like `gh auth login`).
3. Approve the code while logged in to DeskTimers. The terminal picks it up automatically and the TUI opens.
4. deskgit offers to install the DeskTimers git hooks for the repo (see below).

Tokens are scoped per device (revocable in DeskTimers under **Settings → Git Clients**), stored at `~/.config/deskgit/token.json` (0600), and last 90 days — after that the device flow simply runs again. If the API is unreachable but you have a cached token, deskgit continues offline; hooks work fully offline.

## Daily use

| Action | How |
|---|---|
| Pick / clear your current task | `alt+t` — fuzzy-filterable list of your assigned tasks |
| See the current task | Always shown in the status line: `⏱ MOB-101 Fix login redirect` |
| Create a branch | `n` as usual — the input is prefilled with `MOB-101/` when a task is selected |
| Commit | Just commit — the hook prepends `MOB-101: ` unless the message already has a code |
| Push | Untagged commits print a yellow warning (push still succeeds) |

Everything else is stock lazygit — see [docs/](docs/README.md) for the full manual and keybindings.

## Git hooks (`dt-hook`)

deskgit installs two hooks per repo (with your consent — it prompts on repo open):

- **prepare-commit-msg** — prepends the selected task's code to the message. Skips merges, squashes, and amends; never double-prefixes; exits silently on any problem (it will never break a commit).
- **pre-push** — warns (yellow, non-blocking) about pushed commits that carry no task code in the message **and** no code in the branch name. Set `DT_STRICT=1` or `git config desktimers.strictpush true` to make it block instead.

Existing hooks are preserved: they're renamed to `<hook>.local` and chained before ours. Repos using `core.hooksPath` (e.g. Husky) are detected and left alone with a notice. Manual control:

```sh
dt-hook install     # in a repo
dt-hook status
dt-hook uninstall   # restores any chained .local hooks
```

The hooks read the selected task from `<git-dir>/desktimers-task` (per worktree) and send nothing anywhere — all server-side mapping happens through your git provider's webhooks.

## Configuration

`~/.config/deskgit/config.yml` accepts everything lazygit's config does ([docs/Config.md](docs/Config.md)), plus:

```yaml
desktimers:
  apiBaseUrl: https://api-leads.loudowls.com   # point at staging/local if needed
  autoInstallHooks: prompt                 # prompt | always | never
  strictPush: false

keybinding:
  universal:
    desktimersTasks: <a-t>                 # the task-picker key
```

Environment overrides: `DESKTIMERS_API_URL` (API base), `DESKGIT_SKIP_AUTH=1` (skip the auth gate — CI/scripting).

## Development

- All DeskTimers code lives in `pkg/desktimers/` and `cmd/dt-hook/`; patches to upstream lazygit files are deliberately minimal (~10 files) so the fork rebases cleanly.
- The fork is pinned to an upstream release tag (currently `v0.63.1`, remote `upstream`); rebase onto newer tags, not master.
- Build: `sh scripts/build-deskgit.sh [output-dir]` · Test: `go test ./pkg/desktimers/... ./cmd/dt-hook/...` (plus the standard lazygit suite).
- See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) for the upstream codebase guide.

## License & attribution

deskgit is a fork of [lazygit](https://github.com/jesseduffield/lazygit), Copyright © Jesse Duffield, released under the [MIT License](LICENSE). The same license applies to this fork; the DeskTimers integration layer is © LoudOwls. `deskgit --version` reports the upstream lazygit version it is based on.
