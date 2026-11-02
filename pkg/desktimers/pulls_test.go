package desktimers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendPullRequest(t *testing.T) {
	fake := newFakeDeskTick(t)
	code := "CLK-12"

	codes, err := fake.client().SendPullRequest(PullRequestNotice{
		Repo: "acme/clock", Number: 7, Title: "CLK-12/Show the clock", URL: "https://github.com/acme/clock/pull/7",
		State: PullOpen, Draft: true, Head: "feature/CLK-12-clock", Base: "main", TaskCode: &code,
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"CLK-12"}, codes)
	assert.Equal(t, map[string]any{
		"repo": "acme/clock", "number": float64(7), "title": "CLK-12/Show the clock",
		"url": "https://github.com/acme/clock/pull/7", "state": "open", "draft": true,
		"head": "feature/CLK-12-clock", "base": "main", "taskCode": "CLK-12",
	}, fake.body("/api/github/pulls"))
}
