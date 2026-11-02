package desktimers

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens url in the default browser, best-effort.
func OpenBrowser(url string) error {
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
