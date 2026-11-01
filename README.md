# deskgit

**deskgit** is DeskTick's terminal git client. It ties your local git work to your [DeskTick](https://desktick.com) workspace: pick the task you're working on, and every branch and commit you make carries its task code, whichever editor or git client you commit from.

deskgit is a customized fork of the excellent [lazygit](https://github.com/jesseduffield/lazygit) by Jesse Duffield and contributors (MIT). All of lazygit's features work exactly as you know them — deskgit adds a thin DeskTick layer on top, in DeskTick's HUD look,, and installs fully independently of any existing lazygit (own binary name, own `~/.config/deskgit/` config).

## How it works

```
deskgit (TUI)          →  pick a task; branches and commits are prefilled with its code
dt-hook (git hooks)    →  prepare-commit-msg prefixes commits, post-commit tells DeskTick, pre-push stops untagged ones
DeskTick               →  the same tasks, boards and timer the web and desktop apps use
```

Task codes are your workspace's own: `<PROJECT>-<number>`, e.g. `CLK-12`. Because tagging happens in **git hooks**, commits made from VS Code, IntelliJ, or plain `git` on a hooked repo get tagged too — deskgit is the comfortable way to drive it.

deskgit has no server side of its own. It signs in as you and asks the routes the web and desktop apps already use, so it shows what they show and may do what you may do.

> **Not built yet:** DeskTick reading those codes back off GitHub (branches, commits and pull requests showing on the task's card). That is the third step of the GitHub integration; until it lands the codes are in your git history and nowhere else.

## Install

### Homebrew (macOS / Linux)

```sh
brew install debuging-life/tap/deskgit
```

This installs both `deskgit` and its hook companion `dt-hook` from the latest tagged release. Use `brew install --HEAD debuging-life/tap/deskgit` to build the latest `main` branch instead.

### Windows (Scoop)

```powershell
scoop bucket add loudowls https://github.com/debuging-life/homebrew-tap
scoop install deskgit
```

Or download `deskgit_<ver>_windows_amd64.zip` from the [releases page](https://github.com/debuging-life/lazygit/releases) and put both executables on your PATH.

### From source

Requires Go 1.25+:

```sh
git clone git@github.com:debuging-life/lazygit.git deskgit && cd deskgit
git checkout main
sh scripts/build-deskgit.sh ~/.local/bin   # installs `deskgit` and `dt-hook`
```

## First run

1. Run `deskgit` inside any git repo.
2. It opens DeskTick in your browser (and prints the address, in case it does not). Sign in there however you normally do — password, Google, a passkey, a two-step code — and confirm.
3. The browser hands the sign-in back to deskgit on this machine and the TUI opens.
4. In work repos (see `hookOrgs` below) deskgit installs the DeskTick git hooks automatically; other repos are left alone.

This is the desktop app's sign-in (OAuth's authorization-code flow with PKCE and a loopback redirect): what travels through the browser is a one-time code that is useless without a secret that never leaves deskgit. The session is deskgit's own — signing it out leaves your browser signed in, and the other way round. It is kept at `~/.config/deskgit/token.json` (0600). If DeskTick is unreachable and you have a session, deskgit carries on offline; hooks work fully offline.

**Log out / reconnect:** run `deskgit logout` (ends the session on the server and clears it here), or use **Log out of DeskTick** at the bottom of the `alt+t` menu. `deskgit login` runs the sign-in directly; when a session ends mid-use, deskgit offers to sign in again on the spot. Something off? `deskgit doctor` prints ✓/✗ diagnostics (version, config, session, DeskTick reachable, hooks, push mode).

## Daily use

| Action | How |
|---|---|
| Commit | `c` opens the task picker first (live task list, in-progress tasks on top, your current task preselected — Enter-Enter is fast). Picking drops you into the message panel titled `Commit summary — LOUD-124/`: the code is fixed (not part of your text, can't be deleted) and is prepended automatically on confirm — just type the title. Typing your own task code overrides it. Escape in the picker aborts the commit. |
| Create a branch | `n` asks for the branch type first (`feature` / `bugfix` / `hotfix` / `release` / `chore` / `refactor` / `docs` — configurable via `desktimers.branchTypes`), then opens the task picker; the name prompt title shows the fixed `bugfix/LOUD-183-` prefix and you type only the name part — the prefix is prepended on confirm. Escape at any step aborts. |
| Everything in one menu | `alt+d` anywhere, or `d` in the Status panel — the DeskTick menu: pick task, open in browser, clear, check for updates, log out |
| Browse / pre-select a task | `alt+t` anywhere, or `t` while the Status panel is focused — fuzzy-filterable list of your assigned tasks |
| No task yet? | **＋ Create task…** at the bottom of every picker: choose a project, type a title, and the new task is selected on the spot — mid-commit it flows straight back into the commit with the fresh code |
| See the current task | Always on the bottom line, its code on a yellow plate: ` LOUD-124 ` Fix images — with the timer's clock (`▶ 0:42`) when your timer is on that task |
| See your time | The Status panel (`1`): today, this week and your streak, from DeskTick |
| Open a task in the browser | `o` on any highlighted task in the pickers, or the **Open task in browser** item in the `alt+t` menu |
| Commit outside deskgit | The `prepare-commit-msg` hook prepends `LOUD-124/` (your picked task) unless the message already has a code |
| Push | Commits without a task code **block the push** by default (strict mode). One-off escape hatch: `DT_STRICT=0 git push`. Set `desktimers.strictPush: false` for warn-only |
| Open a pull request | `o` in the branches panel as usual — on GitHub, the PR form comes prefilled with the branch's task code as the title and a `Task: <link>` body |

The pick-first steps can be turned off with `desktimers.requireTaskForCommit: false` / `requireTaskForBranch: false`, and the prefix formats customized via `commitPrefixTemplate` (default `{{code}}/`) and `branchPrefixTemplate` (default `{{type}}/{{code}}-`; a template without `{{type}}` skips the branch-type menu) — the git hook always uses the default commit format.

> **macOS note:** if `alt+t` does nothing, your terminal isn't sending Option as Meta. Enable "Use Option as Meta key" (Terminal.app → Settings → Profiles → Keyboard) or set Option to "Esc+" (iTerm2 → Settings → Profiles → Keys), use `t` from the Status panel, or rebind via `keybinding.universal.desktimersTasks` in `~/.config/deskgit/config.yml`.

Everything else is stock lazygit — see [docs/](docs/README.md) for the full manual and keybindings.

## The timer, and your time

deskgit reads the DeskTick timer; it does not start or stop it (the desktop and web apps do).

- The task your timer is on is marked **`▶ TRACKING`** in every picker, sorts to the top, and is preselected — committing while tracking is Enter-Enter. Tasks in a column after the board's first are marked `IN PROGRESS`; tasks in a finished column are not offered.
- The Status panel shows **today**, **this week** and your **streak** (days in a row with at least 15 minutes tracked), against the same goals the desktop app uses (8 hours a day, 40 a week). They are the server's figures, added up across your workspaces and asked for again every minute.
- There is no level here. The desktop app builds its level from every day it has seen on that machine; deskgit can ask for the last 62 days. Two different numbers under one name would be worse than one.

## The look

deskgit wears DeskTick's HUD look: every panel has a bracket at two corners, a numbered head in capitals (`01 STATUS` — the number is still the key that jumps there), and one accent, the yellow of the web app's plates. There is no green in the chrome; green is left to mean "added" in a diff.

It is `gui.border: hud` plus the default `gui.theme`. Set `gui.border` to `rounded` (or `single`, `double`, `bold`, `hidden`) for lazygit's frames, and any `gui.theme` colour to your own. The accent everywhere is `gui.theme.activeBorderColor`.

A terminal cannot be asked what colour its background is, so the greys are mid greys that read on dark and light both. The yellow accent on a **light** terminal is faint: set `activeBorderColor` to something darker there (e.g. `['#8a6d00', bold]`).

## Git hooks (`dt-hook`)

deskgit installs three hooks per repo. By default (`autoInstallHooks: auto`) this happens **silently for work repos only**: when the repository is linked to a project you are on (project settings → Repositories in DeskTick), or its origin remote's owner is in `desktimers.hookOrgs` (default `debuging-life`, `loudowls`), hooks are installed/refreshed and strict-push config synced on repo open; any other repo — or a repo with no origin remote — is left completely untouched. Set `autoInstallHooks: prompt` to be asked per repo instead (`always`/`never` also available).

- **prepare-commit-msg** — prepends the selected task's code to the message and appends a `Task: <link>` trailer (when the task has a webapp link) so the task is one click away from every commit. Skips merges, squashes, amends, `fixup!`/`squash!`/`amend!` messages, and commits git is replaying (rebase, cherry-pick); never duplicates either; exits silently on any problem (it will never break a commit).
- **post-commit** — tells DeskTick of the commit, so it shows on its task's card at once (`POST /api/github/commits`): the repository, sha, branch, message, author, time, your picked task's code, and each changed file's path and line counts — **never the diff**. It returns at once and sends in the background; a commit made offline or signed out waits in `<git-dir>/desktimers-commits/` and is sent on the next push or by deskgit within a minute. DeskTick shows it as "not pushed yet" until GitHub's push webhook confirms it. Commits git is replaying are left to that push.
- **pre-push** — flags pushed commits that carry no task code in the message **and** no code in the branch name. deskgit syncs `git config desktimers.strictpush` into each repo it opens from your `desktimers.strictPush` setting (default **true** = block). Precedence: `DT_STRICT` env (`1`/`true` strict, `0`/`false` off) > repo git config > warn-only default. `DT_STRICT=0 git push` pushes anyway this once.

Existing hooks are preserved: they're renamed to `<hook>.local` and chained before ours. Repos using `core.hooksPath` (e.g. Husky) are detected and left alone with a notice. Manual control:

```sh
dt-hook install     # in a repo
dt-hook status
dt-hook uninstall   # restores any chained .local hooks
```

The hooks read the selected task from `<git-dir>/desktimers-task` (per worktree). Only post-commit sends anything, and only to your DeskTick, as you.

## Configuration

`~/.config/deskgit/config.yml` accepts everything lazygit's config does ([docs/Config.md](docs/Config.md)), plus:

```yaml
desktimers:
  url: https://dev.desktick.com            # the DeskTick web app; the API is the same address under /api
  autoInstallHooks: auto                   # auto | prompt | always | never
  hookOrgs:                                # "work" owners for auto mode
    - debuging-life
    - loudowls
  strictPush: true                         # block pushes with untagged commits
  checkForUpdates: true                    # update notice, max once/24h;
                                           # Homebrew installs get an "Update now" button
  requireTaskForCommit: true               # commit opens the task picker first
  requireTaskForBranch: true               # new branch: type menu + task picker
  commitPrefixTemplate: '{{code}}/'        # readonly prefix on commit messages
  branchPrefixTemplate: '{{type}}/{{code}}-' # omit {{type}} to skip the type menu
  branchTypes: [feature, bugfix, hotfix, release, chore, refactor, docs]

keybinding:
  universal:
    desktimersTasks: <a-t>                 # the task-picker key
```

Environment overrides: `DESKTICK_URL` (the DeskTick address for one run, e.g. `DESKTICK_URL=http://localhost:5174 deskgit` against a local web app), `DESKGIT_SKIP_AUTH=1` (skip the auth gate — CI/scripting).

## Development

- All DeskTick code lives in `pkg/desktimers/`, `cmd/dt-hook/` and `pkg/gui/controllers/helpers/desktimers_*.go`; patches to upstream lazygit files are deliberately minimal so the fork rebases cleanly. The HUD frame is one flag in `pkg/gocui` (`Gui.Brackets`). The package and the config key are still called `desktimers`: renaming the key would break every config already written.
- The fork is pinned to an upstream release tag (currently `v0.63.1`, remote `upstream`); rebase onto newer tags, not master.
- Build: `sh scripts/build-deskgit.sh [output-dir]` · Test: `go test ./pkg/desktimers/... ./cmd/dt-hook/...` (plus the standard lazygit suite).
- See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) for the upstream codebase guide.

## License & attribution

deskgit is a fork of [lazygit](https://github.com/jesseduffield/lazygit), Copyright © Jesse Duffield, released under the [MIT License](LICENSE). The same license applies to this fork; the DeskTick integration layer is © LoudOwls. `deskgit --version` reports the upstream lazygit version it is based on.
