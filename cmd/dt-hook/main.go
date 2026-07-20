// dt-hook is the git-hook companion binary for deskgit (DeskTimers). It is
// invoked by the prepare-commit-msg and pre-push hooks that deskgit installs,
// and can also install/uninstall those hooks itself.
//
// Design rule: hook subcommands must NEVER break git. Any internal failure
// exits 0 silently; only strict mode (DT_STRICT=1 or desktimers.strictpush)
// intentionally blocks a push.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "prepare-commit-msg":
		runPrepareCommitMsg(os.Args[2:])
	case "pre-push":
		os.Exit(runPrePush(os.Args[2:], os.Stdin, os.Stderr, "."))
	case "install":
		if err := runInstall(); err != nil {
			fmt.Fprintf(os.Stderr, "dt-hook: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("DeskTimers git hooks installed.")
	case "uninstall":
		if err := desktimers.UninstallHooks("."); err != nil {
			fmt.Fprintf(os.Stderr, "dt-hook: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("DeskTimers git hooks removed.")
	case "status":
		status, err := desktimers.HooksStatus(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "dt-hook: %v\n", err)
			os.Exit(1)
		}
		switch status {
		case desktimers.HooksInstalled:
			fmt.Println("installed")
		case desktimers.HooksOutdated:
			fmt.Println("outdated")
		default:
			fmt.Println("missing")
		}
	case "version":
		fmt.Println("dt-hook " + desktimers.Version)
	default:
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `usage: dt-hook <command>

commands:
  prepare-commit-msg <msgfile> [source] [sha]   (invoked by git)
  pre-push <remote-name> <remote-url>           (invoked by git)
  install     install DeskTimers hooks into the current repo
  uninstall   remove DeskTimers hooks from the current repo
  status      report hook install state (installed|outdated|missing)
  version     print version`)
}

func runInstall() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving dt-hook path: %w", err)
	}
	return desktimers.InstallHooks(".", exe)
}

// runPrepareCommitMsg prefixes the commit message with the selected task
// code. Every failure path returns silently: a broken hook must not block
// commits.
func runPrepareCommitMsg(args []string) {
	if len(args) < 1 {
		return
	}
	msgFile := args[0]
	source := ""
	if len(args) > 1 {
		source = args[1]
	}
	state, err := desktimers.LoadState(".")
	if err != nil || state == nil || state.Code == "" {
		return
	}
	data, err := os.ReadFile(msgFile)
	if err != nil {
		return
	}
	rewritten, changed := desktimers.PrefixCommitMessage(string(data), state.Code, source)
	if !changed {
		return
	}
	_ = os.WriteFile(msgFile, []byte(rewritten), 0o644)
}
