package hosting_service

import "github.com/jesseduffield/lazygit/pkg/desktimers"

// deskgit: this file is the fork's own, so a rebase onto lazygit never
// conflicts on it.

// resolveSSHHost is the host an SSH alias stands for; a variable so tests
// need no ssh.
var resolveSSHHost = desktimers.ResolveSSHHost
