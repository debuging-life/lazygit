package git_commands

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/stretchr/testify/assert"
)

func TestFetchedPullRequestsSayWhereTheyStand(t *testing.T) {
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		asked = string(body)
		_, _ = w.Write([]byte(`{"data":{"repository":{"a1":{"edges":[
			{"node":{"headRefName":"feature/CLK-1-x","number":7,"state":"OPEN",
			 "reviewDecision":null,"mergeable":"CONFLICTING",
			 "latestOpinionatedReviews":{"nodes":[{"state":"APPROVED"}]},
			 "headRepositoryOwner":{"login":"o"}}},
			{"node":{"headRefName":"feature/CLK-1-x","number":6,"state":"OPEN",
			 "reviewDecision":"CHANGES_REQUESTED","mergeable":"MERGEABLE",
			 "latestOpinionatedReviews":{"nodes":[]},
			 "headRepositoryOwner":{"login":"o"}}}
		]}}}}`))
	}))
	defer server.Close()

	prs, err := (&GitHubCommands{}).fetchRecentPRsAux(server.URL, "o", "r", []string{"feature/CLK-1-x"}, "token")
	assert.NoError(t, err)
	if !assert.Len(t, prs, 2) {
		return
	}

	assert.Equal(t, desktimers.ReviewApproved, prs[0].Review)
	assert.True(t, prs[0].Conflicting)
	assert.Equal(t, desktimers.ReviewChangesRequested, prs[1].Review)
	assert.False(t, prs[1].Conflicting)
	for _, field := range []string{"reviewDecision", "mergeable", "latestOpinionatedReviews"} {
		assert.True(t, strings.Contains(asked, field), "the query asks for %s", field)
	}
}
