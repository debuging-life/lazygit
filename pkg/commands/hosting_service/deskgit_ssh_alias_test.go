package hosting_service

import (
	"os"
	"testing"

	"github.com/jesseduffield/lazygit/pkg/fakes"
	"github.com/jesseduffield/lazygit/pkg/i18n"
	"github.com/stretchr/testify/assert"
)

// TestMain keeps every test here off the machine's own ~/.ssh/config: no
// name is an alias unless a test says so.
func TestMain(m *testing.M) {
	resolveSSHHost = func(alias string) string { return alias }
	os.Exit(m.Run())
}

func stubSSHAliases(t *testing.T, aliases map[string]string) {
	t.Helper()
	original := resolveSSHHost
	resolveSSHHost = func(alias string) string {
		if host, ok := aliases[alias]; ok {
			return host
		}
		return alias
	}
	t.Cleanup(func() { resolveSSHHost = original })
}

func TestGetServiceInfoThroughAnSSHAlias(t *testing.T) {
	stubSSHAliases(t, map[string]string{"github-work": "github.com"})
	mgr := NewHostingServiceMgr(&fakes.FakeFieldLogger{}, i18n.EnglishTranslationSet(),
		"git@github-work:acme/clock.git", nil)

	info, err := mgr.GetServiceInfo()

	problem := ""
	if err != nil {
		problem = err.Error()
	}
	assert.Equal(t, ServiceInfo{
		Provider: "github", WebDomain: "github.com", Owner: "acme", Repository: "clock", RepoName: "acme/clock",
	}, info)
	assert.Empty(t, problem)
}
