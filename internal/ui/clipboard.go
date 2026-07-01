package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func writeClipboard(value string) error {
	switch runtime.GOOS {
	case "darwin":
		return runClipboardCommand(value, "pbcopy")
	case "windows":
		return runClipboardCommand(value, "clip")
	default:
		candidates := [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
		for _, args := range candidates {
			if _, err := exec.LookPath(args[0]); err != nil {
				continue
			}
			return runClipboardCommand(value, args...)
		}
		return fmt.Errorf("no clipboard command found (tried wl-copy, xclip, xsel)")
	}
}

func runClipboardCommand(value string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
