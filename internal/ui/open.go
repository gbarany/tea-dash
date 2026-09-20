package ui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// openURL opens url in the user's default browser.
func openURL(rawURL string) error {
	cmd, err := browserCommand(rawURL, runtime.GOOS)
	if err != nil {
		return err
	}
	return cmd.Start()
}

// browserCommand accepts only absolute web URLs from API responses. In particular,
// local files, custom protocol handlers, and opener options must not be launched.
func browserCommand(rawURL, platform string) (*exec.Cmd, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("cannot open URL: expected an absolute HTTP(S) URL")
	}
	var name string
	var args []string
	switch platform {
	case "darwin":
		name = "open"
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default: // linux, bsd, ...
		name = "xdg-open"
	}
	args = append(args, rawURL)
	// #nosec G204 -- executable and prefix arguments are fixed above; the only dynamic argument is a validated absolute HTTP(S) URL, never shell source or an option.
	return exec.Command(name, args...), nil
}
