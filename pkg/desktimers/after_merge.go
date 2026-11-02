package desktimers

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CommitsOnlyHere counts the commits on a local branch that its branch on
// the remote does not have. A pull request merged by squashing or
// rebasing leaves none of the branch's commits on the base, so git's own
// "is it merged?" says no for every one of them: what decides whether the
// local branch can go is whether anything on it never reached GitHub.
// A remote branch that is gone is an error, not zero: there is then
// nothing to compare with.
func CommitsOnlyHere(repoPath string, local string, remoteRef string) (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", "refs/remotes/"+remoteRef+"..refs/heads/"+local, "--")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("comparing %s with %s: %w", local, remoteRef, err)
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
