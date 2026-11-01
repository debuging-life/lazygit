package hosting_service

import (
	"maps"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

// deskgit: this file is the fork's own, so a rebase onto lazygit never
// conflicts on it.

// resolveSSHHost is the host an SSH alias stands for; a variable so tests
// need no ssh.
var resolveSSHHost = desktimers.ResolveSSHHost

// sshEndpoints are host names a service takes SSH on besides its own, for
// the networks that block port 22.
var sshEndpoints = map[string]string{
	"ssh.github.com":    "github.com",
	"altssh.gitlab.com": "gitlab.com",
}

// withSSHAlias adds the remote's SSH alias to the configured services when
// it stands for a known one: git@github-work:acme/clock.git, with
// "Host github-work / HostName github.com" in ~/.ssh/config, is GitHub's.
// A remote that already names a service, or an alias configured by hand,
// is left as it is, and ssh is not asked.
func withSSHAlias(remoteURL string, services map[string]string) map[string]string {
	alias := desktimers.SSHRemoteHost(remoteURL)
	if alias == "" {
		return services
	}
	for _, domain := range defaultServiceDomains {
		if strings.Contains(remoteURL, domain.gitDomain) {
			return services
		}
	}
	for gitDomain := range services {
		if strings.Contains(remoteURL, gitDomain) {
			return services
		}
	}

	host := strings.ToLower(resolveSSHHost(alias))
	if endpoint, ok := sshEndpoints[host]; ok {
		host = endpoint
	}
	if host == "" || host == strings.ToLower(alias) {
		return services
	}

	service := ""
	if configured, ok := services[host]; ok {
		service = configured
	} else {
		for _, domain := range defaultServiceDomains {
			if domain.gitDomain == host {
				service = domain.serviceDefinition.provider + ":" + domain.webDomain
				break
			}
		}
	}
	if service == "" {
		return services
	}
	withAlias := maps.Clone(services)
	if withAlias == nil {
		withAlias = map[string]string{}
	}
	withAlias[alias] = service
	return withAlias
}
