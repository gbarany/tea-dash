package ui

import (
	"os/exec"
	"runtime"
)

// openURL opens url in the user's default browser.
func openURL(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default: // linux, bsd, ...
		name = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(name, args...).Start()
}
