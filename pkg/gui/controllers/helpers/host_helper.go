package helpers

import (
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands/hosting_service"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

// this helper just wraps our hosting_service package

type HostHelper struct {
	c *HelperCommon
}

func NewHostHelper(
	c *HelperCommon,
) *HostHelper {
	return &HostHelper{
		c: c,
	}
}

func (self *HostHelper) GetPullRequestURL(from string, to string) (string, error) {
	mgr, err := self.getHostingServiceMgr()
	if err != nil {
		return "", err
	}
	prURL, err := mgr.GetPullRequestURL(from, to)
	if err != nil {
		return "", err
	}

	// deskgit: GitHub PRs from task-coded branches get the title (and, when
	// the deep link resolves, body) prefilled. Best effort — the title needs
	// no network; the body lookup silently skips on failure.
	if code := desktimers.ExtractCode(from); code != "" && strings.HasPrefix(prURL, "https://github.com/") {
		prURL = desktimers.AugmentGitHubPullRequestURL(prURL, from, desktimers.TaskURLForCode(".", code))
	}
	return prURL, nil
}

func (self *HostHelper) GetCommitURL(commitHash string) (string, error) {
	mgr, err := self.getHostingServiceMgr()
	if err != nil {
		return "", err
	}
	return mgr.GetCommitURL(commitHash)
}

// getting this on every request rather than storing it in state in case our remoteURL changes
// from one invocation to the next.
func (self *HostHelper) getHostingServiceMgr() (*hosting_service.HostingServiceMgr, error) {
	remoteUrl, err := self.c.Git().Remote.GetRemoteURL("origin")
	if err != nil {
		return nil, err
	}
	configServices := self.c.UserConfig().Services
	return hosting_service.NewHostingServiceMgr(self.c.Log, self.c.Tr, remoteUrl, configServices), nil
}
