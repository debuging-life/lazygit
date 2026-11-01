package desktimers

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SSHRemoteHost is the host a remote is reached through over SSH, as it is
// written in the remote ("github-work" in git@github-work:acme/clock.git),
// or "" for a remote that is not SSH.
func SSHRemoteHost(remoteURL string) string {
	if rest, ok := strings.CutPrefix(remoteURL, "ssh://"); ok {
		host, _, _ := strings.Cut(rest, "/")
		if _, after, found := strings.Cut(host, "@"); found {
			host = after
		}
		// A port, or an IPv6 address in brackets, follows the name.
		if strings.HasPrefix(host, "[") {
			end := strings.Index(host, "]")
			if end < 0 {
				return ""
			}
			return host[1:end]
		}
		host, _, _ = strings.Cut(host, ":")
		return host
	}
	if strings.Contains(remoteURL, "://") {
		return ""
	}
	// scp-like: [user@]host:path, where the host has no slash and is more
	// than a drive letter (C:\repo is a path).
	before, _, found := strings.Cut(remoteURL, ":")
	if !found || strings.ContainsAny(before, `/\`) {
		return ""
	}
	if _, after, found := strings.Cut(before, "@"); found {
		before = after
	}
	if len(before) < 2 {
		return ""
	}
	return before
}

var (
	sshHostsMutex sync.Mutex
	sshHosts      = map[string]string{}
)

// sshHostname asks ssh which host an alias in ~/.ssh/config stands for.
var sshHostname = func(alias string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// "--" so an alias can never be read as an option.
	out, err := exec.CommandContext(ctx, "ssh", "-G", "--", alias).Output()
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "hostname "); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// ResolveSSHHost is the host an SSH alias stands for (the HostName of its
// Host block in ~/.ssh/config), or the alias itself when it is not one, or
// "" when ssh cannot say. Asked once per alias while deskgit runs.
func ResolveSSHHost(alias string) string {
	if alias == "" || strings.HasPrefix(alias, "-") {
		return ""
	}
	sshHostsMutex.Lock()
	defer sshHostsMutex.Unlock()
	if host, ok := sshHosts[alias]; ok {
		return host
	}
	host := sshHostname(alias)
	sshHosts[alias] = host
	return host
}
