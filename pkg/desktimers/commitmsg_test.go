package desktimers

import "testing"

func TestPrefixCommitMessage(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		code        string
		source      string
		want        string
		wantChanged bool
	}{
		{
			name:        "plain -m message",
			content:     "fix login redirect\n",
			code:        "MOB-101",
			want:        "MOB-101: fix login redirect\n",
			wantChanged: true,
		},
		{
			name:        "template with comments only",
			content:     "\n# Please enter the commit message for your changes.\n# Lines starting with '#' will be ignored.\n",
			code:        "MOB-101",
			want:        "MOB-101: \n# Please enter the commit message for your changes.\n# Lines starting with '#' will be ignored.\n",
			wantChanged: true,
		},
		{
			name:        "message already contains a code",
			content:     "DES-9: fix header\n",
			code:        "MOB-101",
			want:        "DES-9: fix header\n",
			wantChanged: false,
		},
		{
			name:        "code only in comments still gets prefixed",
			content:     "fix header\n# On branch MOB-101/fix-header\n",
			code:        "MOB-101",
			want:        "MOB-101: fix header\n# On branch MOB-101/fix-header\n",
			wantChanged: true,
		},
		{
			name:        "merge source skipped",
			content:     "Merge branch 'dev'\n",
			code:        "MOB-101",
			source:      "merge",
			want:        "Merge branch 'dev'\n",
			wantChanged: false,
		},
		{
			name:        "squash source skipped",
			content:     "squash! something\n",
			code:        "MOB-101",
			source:      "squash",
			want:        "squash! something\n",
			wantChanged: false,
		},
		{
			name:        "amend (commit source) skipped",
			content:     "older message\n",
			code:        "MOB-101",
			source:      "commit",
			want:        "older message\n",
			wantChanged: false,
		},
		{
			name:        "no code selected",
			content:     "fix login redirect\n",
			code:        "",
			want:        "fix login redirect\n",
			wantChanged: false,
		},
		{
			name:        "message source (-m) still prefixed",
			content:     "fix login redirect\n",
			code:        "MOB-101",
			source:      "message",
			want:        "MOB-101: fix login redirect\n",
			wantChanged: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, changed := PrefixCommitMessage(test.content, test.code, test.source)
			if got != test.want || changed != test.wantChanged {
				t.Errorf("PrefixCommitMessage(%q, %q, %q) = (%q, %v), want (%q, %v)",
					test.content, test.code, test.source, got, changed, test.want, test.wantChanged)
			}
		})
	}
}
