package desktimers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPullRequestReview(t *testing.T) {
	scenarios := []struct {
		name     string
		decision string
		opinions []string
		expected string
	}{
		{"nobody has reviewed", "", nil, ""},
		{"GitHub's verdict wins", ReviewWaiting, []string{ReviewApproved}, ReviewWaiting},
		{"an approval", "", []string{ReviewApproved}, ReviewApproved},
		{"changes requested after an approval", "", []string{ReviewApproved, ReviewChangesRequested}, ReviewChangesRequested},
		{"changes requested before an approval", "", []string{ReviewChangesRequested, ReviewApproved}, ReviewChangesRequested},
		{"comments are not a verdict", "", []string{"COMMENTED"}, ""},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			assert.Equal(t, s.expected, PullRequestReview(s.decision, s.opinions))
		})
	}
}
