package desktimers

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens url in the default browser, best-effort. Only a web
// address is handed on: each platform's opener would as readily run a
// file or a program it was given.
func OpenBrowser(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("not a web address: %s", url)
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		// Not cmd /c start: cmd reads the & between query parameters as
		// the end of the command, and the sign-in address has several.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
