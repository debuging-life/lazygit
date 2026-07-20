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
4. In work-org repos (see `hookOrgs` below) deskgit installs the DeskTimers git hooks automatically; other repos are left alone.

Tokens are scoped per device (revocable in DeskTimers under **Settings → Git Clients**), stored at `~/.config/deskgit/token.json` (0600), and last 90 days — after that the device flow simply runs again. If the API is unreachable but you have a cached token, deskgit continues offline; hooks work fully offline.

**Log out / reconnect:** run `deskgit logout` (revokes the device token server-side and clears the local login), or use the **Log out of DeskTimers** item at the bottom of the `alt+t` menu. `deskgit login` re-runs the device flow directly; when a login expires mid-session, deskgit offers to re-login on the spot. Something off? `deskgit doctor` prints ✓/✗ diagnostics (version, config, token, API reachability, hooks, push mode).

## Daily use

| Action | How |
|---|---|
| Commit | `c` opens the task picker first (live task list, in-progress tasks on top, your current task preselected — Enter-Enter is fast). Picking drops you into the message panel titled `Commit summary — LOUD-124/`: the code is fixed (not part of your text, can't be deleted) and is prepended automatically on confirm — just type the title. Typing your own task code overrides it. Escape in the picker aborts the commit. |
| Create a branch | `n` asks for the branch type first (`feature` / `bugfix` / `hotfix` / `release` / `chore` / `refactor` / `docs` — configurable via `desktimers.branchTypes`), then opens the task picker; the name prompt title shows the fixed `bugfix/LOUD-183-` prefix and you type only the name part — the prefix is prepended on confirm. Escape at any step aborts. |
| Browse / pre-select a task | `alt+t` anywhere, or `t` while the Status panel is focused — fuzzy-filterable list of your assigned tasks |
| See the current task | Always shown in the status line: `⏱ LOUD-124 Fix images` |
| Open a task in the browser | `o` on any highlighted task in the pickers, or the **Open task in browser** item in the `alt+t` menu |
| Commit outside deskgit | The `prepare-commit-msg` hook prepends `LOUD-124/` (your picked task) unless the message already has a code |
| Push | Commits without a task code **block the push** by default (strict mode). One-off escape hatch: `DT_STRICT=0 git push`. Set `desktimers.strictPush: false` for warn-only |

The pick-first steps can be turned off with `desktimers.requireTaskForCommit: false` / `requireTaskForBranch: false`, and the prefix formats customized via `commitPrefixTemplate` (default `{{code}}/`) and `branchPrefixTemplate` (default `{{type}}/{{code}}-`; a template without `{{type}}` skips the branch-type menu) — the git hook always uses the default commit format.

> **macOS note:** if `alt+t` does nothing, your terminal isn't sending Option as Meta. Enable "Use Option as Meta key" (Terminal.app → Settings → Profiles → Keyboard) or set Option to "Esc+" (iTerm2 → Settings → Profiles → Keys), use `t` from the Status panel, or rebind via `keybinding.universal.desktimersTasks` in `~/.config/deskgit/config.yml`.

Everything else is stock lazygit — see [docs/](docs/README.md) for the full manual and keybindings.

## Git hooks (`dt-hook`)

deskgit installs two hooks per repo. By default (`autoInstallHooks: auto`) this happens **silently for work repos only**: when the origin remote's owner is in `desktimers.hookOrgs` (default `debuging-life`, `loudowls`), hooks are installed/refreshed and strict-push config synced on repo open; any other repo — or a repo with no origin remote — is left completely untouched. Set `autoInstallHooks: prompt` to be asked per repo instead (`always`/`never` also available).

- **prepare-commit-msg** — prepends the selected task's code to the message. Skips merges, squashes, and amends; never double-prefixes; exits silently on any problem (it will never break a commit).
- **pre-push** — flags pushed commits that carry no task code in the message **and** no code in the branch name. deskgit syncs `git config desktimers.strictpush` into each repo it opens from your `desktimers.strictPush` setting (default **true** = block). Precedence: `DT_STRICT` env (`1`/`true` strict, `0`/`false` off) > repo git config > warn-only default. `DT_STRICT=0 git push` pushes anyway this once.

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
  autoInstallHooks: auto                   # auto | prompt | always | never
  hookOrgs:                                # "work" owners for auto mode
    - debuging-life
    - loudowls
  strictPush: true                         # block pushes with untagged commits
  checkForUpdates: true                    # brew-tap update notice, max once/24h

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
