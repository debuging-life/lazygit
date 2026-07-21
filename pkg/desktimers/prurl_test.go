package desktimers

import "testing"

func TestAugmentGitHubPullRequestURL(t *testing.T) {
	ghURL := "https://github.com/loudowls/deskgit/compare/feature%2FLOUD-124-images?expand=1"
	deepLink := "https://leads.desktimers.com/t/LOUD-124"

	tests := []struct {
		name    string
		prURL   string
		branch  string
		taskURL string
		want    string
	}{
		{
			name:    "github with code and deep link",
			prURL:   ghURL,
			branch:  "feature/LOUD-124-images",
			taskURL: deepLink,
			want:    ghURL + "&title=LOUD-124%2F&body=Task%3A+" + "https%3A%2F%2Fleads.desktimers.com%2Ft%2FLOUD-124",
		},
		{
			name:   "github with code, no deep link (title only)",
			prURL:  ghURL,
			branch: "feature/LOUD-124-images",
			want:   ghURL + "&title=LOUD-124%2F",
		},
		{
			name:   "github URL without existing query uses ?",
			prURL:  "https://github.com/loudowls/deskgit/pull/new/branch",
			branch: "LOUD-9/x",
			want:   "https://github.com/loudowls/deskgit/pull/new/branch?title=LOUD-9%2F",
		},
		{
			name:   "codeless branch untouched",
			prURL:  ghURL,
			branch: "feature/no-task-here",
			want:   ghURL,
		},
		{
			name:    "non-github host untouched",
			prURL:   "https://gitlab.com/g/r/-/merge_requests/new?merge_request%5Bsource_branch%5D=x",
			branch:  "feature/LOUD-124-images",
			taskURL: deepLink,
			want:    "https://gitlab.com/g/r/-/merge_requests/new?merge_request%5Bsource_branch%5D=x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AugmentGitHubPullRequestURL(tt.prURL, tt.branch, tt.taskURL); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}
