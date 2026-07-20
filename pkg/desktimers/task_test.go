package desktimers

import "testing"

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain code", "MOB-101", "MOB-101"},
		{"code in message", "MOB-101: fix login redirect", "MOB-101"},
		{"lowercase", "mob-101 fix things", "MOB-101"},
		{"short project code", "dt-12", "DT-12"},
		{"alphanumeric project code", "A1B2-33 change", "A1B2-33"},
		{"embedded in branch name", "feature/mob-101-fix-login", "MOB-101"},
		{"branch prefix style", "DES-123/fix-header", "DES-123"},
		{"no match", "fix login redirect", ""},
		{"dash without number", "MOB- broken", ""},
		{"multiple matches returns first", "MOB-101 relates to DES-9", "MOB-101"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ExtractCode(test.input); got != test.want {
				t.Errorf("ExtractCode(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestBranchPrefix(t *testing.T) {
	if got := BranchPrefix("MOB-101"); got != "MOB-101/" {
		t.Errorf("BranchPrefix(MOB-101) = %q, want %q", got, "MOB-101/")
	}
}
