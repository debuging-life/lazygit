package desktimers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceExecutableMovesTheRunningOneAside(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "deskgit.exe")
	write := func(path string, content string) {
		t.Helper()
		assert.NoError(t, os.WriteFile(path, []byte(content), 0o755))
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		assert.NoError(t, err)
		return string(data)
	}
	aside := func() []string {
		t.Helper()
		paths, _ := filepath.Glob(dest + ".*.old")
		contents := []string{}
		for _, path := range paths {
			contents = append(contents, read(path))
		}
		return contents
	}
	write(dest, "v1")
	write(filepath.Join(dir, "new"), "v2")

	assert.NoError(t, replaceExecutable(filepath.Join(dir, "new"), dest, true))
	assert.Equal(t, "v2", read(dest))
	assert.Equal(t, []string{"v1"}, aside())

	// The next update clears away what the last one left.
	write(filepath.Join(dir, "newer"), "v3")
	assert.NoError(t, replaceExecutable(filepath.Join(dir, "newer"), dest, true))
	assert.Equal(t, "v3", read(dest))
	assert.Equal(t, []string{"v2"}, aside())
}
