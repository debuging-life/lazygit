package desktimers

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSHRemoteHost(t *testing.T) {
	for remote, host := range map[string]string{
		"git@github-work:acme/clock.git":            "github-work",
		"github-work:acme/clock.git":                "github-work",
		"ssh://git@github-work/acme/clock.git":      "github-work",
		"ssh://git@github-work:2222/acme/clock.git": "github-work",
		"ssh://git@[::1]:2222/acme/clock.git":       "::1",
		"https://github.com/acme/clock.git":         "",
		"file:///srv/clock.git":                     "",
		"/srv/clock.git":                            "",
		"./clock:old":                               "",
		`C:\repos\clock`:                            "",
		"":                                          "",
	} {
		assert.Equal(t, host, SSHRemoteHost(remote), remote)
	}
}

func stubSSHHostname(t *testing.T, hosts map[string]string) *int {
	t.Helper()
	asked := 0
	original := sshHostname
	sshHostname = func(alias string) string {
		asked++
		return hosts[alias]
	}
	sshHosts = map[string]string{}
	t.Cleanup(func() {
		sshHostname = original
		sshHosts = map[string]string{}
	})
	return &asked
}

func TestResolveSSHHostAsksOncePerAlias(t *testing.T) {
	asked := stubSSHHostname(t, map[string]string{"github-work": "github.com"})

	assert.Equal(t, "github.com", ResolveSSHHost("github-work"))
	assert.Equal(t, "github.com", ResolveSSHHost("github-work"))
	// An answer of nothing is kept too, so a missing ssh is asked once.
	assert.Equal(t, "", ResolveSSHHost("nowhere"))
	assert.Equal(t, "", ResolveSSHHost("nowhere"))

	assert.Equal(t, 2, *asked)
}

func TestResolveSSHHostRefusesAnOption(t *testing.T) {
	asked := stubSSHHostname(t, nil)

	assert.Equal(t, "", ResolveSSHHost("-oProxyCommand=touch /tmp/x"))
	assert.Equal(t, 0, *asked)
}

func TestSSHHostnameAsksSSH(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh is needed")
	}
	// A name with no Host block of its own is its own host name. ssh
	// lowercases it.
	assert.Equal(t, "no-such-alias.deskgit.invalid", sshHostname("No-Such-Alias.deskgit.invalid"))
}
