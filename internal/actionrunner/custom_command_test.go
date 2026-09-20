package actionrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCustomCommandWindowsConfiguredShell(t *testing.T) {
	for _, configured := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:/Program Files/Git/usr/bin/sh.exe`,
		`C:\Program Files\Git\bin\BASH.EXE`,
	} {
		t.Run(configured, func(t *testing.T) {
			t.Setenv("SHELL", configured)
			cmd := customCommandProcess("printf ok", "repo", "windows")
			if cmd.Args[0] != configured {
				t.Fatalf("shell = %q, want configured %q", cmd.Args[0], configured)
			}
			if cmd.Dir != "repo" || len(cmd.Args) != 3 || cmd.Args[1] != "-c" || cmd.Args[2] != "printf ok" {
				t.Fatalf("unexpected command: %+v", cmd)
			}
		})
	}
}

func TestCustomCommandIgnoresIncompatibleShells(t *testing.T) {
	for _, platform := range []string{"windows", "darwin", "linux"} {
		for _, configured := range []string{"", "fish", `C:\Windows\System32\cmd.exe`, "pwsh.exe", "zsh", "bash.exe -i"} {
			t.Run(platform+"/"+configured, func(t *testing.T) {
				t.Setenv("SHELL", configured)
				if got := customCommandProcess("printf ok", "", platform).Args[0]; got != "sh" {
					t.Fatalf("shell = %q, want sh", got)
				}
			})
		}
	}
}

func TestCustomCommandWindowsShellOutsidePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a Unix shell symlink; Windows paths are covered separately")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	configured := filepath.Join(dir, "bash.exe")
	if err := os.Symlink(sh, configured); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", configured)
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("sh"); err == nil {
		t.Fatal("fixture unexpectedly has sh on PATH")
	}
	output, err := customCommandProcess("printf shell-ok", dir, "windows").Output()
	if err != nil {
		t.Fatalf("configured shell outside PATH: %v", err)
	}
	if string(output) != "shell-ok" {
		t.Fatalf("output = %q, want shell-ok", output)
	}
}
