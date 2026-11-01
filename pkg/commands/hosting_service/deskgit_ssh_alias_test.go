package hosting_service

import (
	"os"
	"testing"
)

// TestMain keeps every test here off the machine's own ~/.ssh/config: no
// name is an alias unless a test says so.
func TestMain(m *testing.M) {
	resolveSSHHost = func(alias string) string { return alias }
	os.Exit(m.Run())
}
