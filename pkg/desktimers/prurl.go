package desktimers

import (
	"net/url"
	"strings"
	"time"
)

// AugmentGitHubPullRequestURL appends PR-prefill query params for a
// task-coded branch: `title=<CODE>/` and, when a task deep link is known,
// `body=Task: <link>`. Non-GitHub URLs and codeless branches pass through
// untouched.
func AugmentGitHubPullRequestURL(prURL string, branchName string, taskURL string) string {
	if !strings.HasPrefix(prURL, "https://github.com/") {
		return prURL
	}
	code := ExtractCode(branchName)
	if code == "" {
		return prURL
	}

	separator := "?"
	if strings.Contains(prURL, "?") {
		separator = "&"
	}
	augmented := prURL + separator + "title=" + url.QueryEscape(code+"/")
	if taskURL != "" {
		augmented += "&body=" + url.QueryEscape("Task: "+taskURL)
	}
	return augmented
}

// TaskURLForCode resolves a task code to its webapp deep link: the selected
// task's state file first (free), then a short API lookup. Best effort — ""
// on any failure or no match.
func TaskURLForCode(repoPath string, code string) string {
	if state, err := LoadState(repoPath); err == nil && state != nil && state.Code == code && state.URL != "" {
		return state.URL
	}

	client, err := NewClientFromToken()
	if err != nil {
		return ""
	}
	client.HTTPClient.Timeout = 3 * time.Second
	tasks, err := client.GetTasks("active")
	if err != nil {
		return ""
	}
	for _, task := range tasks {
		if task.Code == code {
			return task.URL
		}
	}
	return ""
}
