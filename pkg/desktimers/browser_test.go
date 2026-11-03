package desktimers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenBrowserHandsOnOnlyAWebAddress(t *testing.T) {
	for _, target := range []string{"file:///C:/x.exe", `\\host\share\x.exe`, "/usr/bin/true", "javascript:alert(1)"} {
		assert.Error(t, OpenBrowser(target), target)
	}
}
