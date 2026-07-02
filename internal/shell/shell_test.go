package shell

import (
	"bytes"
	"reflect"
	"testing"
)

func TestBuildCommandUsesShellCWithStdinAndDir(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	cmd := BuildCommand("delta --paging=always", []byte("diff"), "/tmp/repo")

	if cmd.Name != "/bin/zsh" {
		t.Fatalf("Name = %q, want /bin/zsh", cmd.Name)
	}
	if want := []string{"-c", "delta --paging=always"}; !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, want)
	}
	if !bytes.Equal(cmd.Stdin, []byte("diff")) {
		t.Fatalf("Stdin = %q, want diff bytes", string(cmd.Stdin))
	}
	if cmd.Dir != "/tmp/repo" {
		t.Fatalf("Dir = %q, want /tmp/repo", cmd.Dir)
	}
}

func TestBuildCommandDefaultsToBinSh(t *testing.T) {
	t.Setenv("SHELL", "")

	cmd := BuildCommand("less -R", nil, "")

	if cmd.Name != "/bin/sh" {
		t.Fatalf("Name = %q, want /bin/sh", cmd.Name)
	}
	if want := []string{"-c", "less -R"}; !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, want)
	}
}

func TestBuildExecCommandUsesShellCDirAndOptionalStdin(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	cmd := BuildExecCommand("lazygit", []byte("input"), "/tmp/repo")

	if cmd.Path != "/bin/zsh" {
		t.Fatalf("Path = %q, want /bin/zsh", cmd.Path)
	}
	if want := []string{"/bin/zsh", "-c", "lazygit"}; !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Dir != "/tmp/repo" {
		t.Fatalf("Dir = %q, want /tmp/repo", cmd.Dir)
	}
	if cmd.Stdin == nil {
		t.Fatal("Stdin should be populated when stdin bytes are provided")
	}
	if cmd.Stdout != nil || cmd.Stderr != nil {
		t.Fatal("Stdout/Stderr should stay nil so Bubble Tea ExecProcess can wire terminal output")
	}
}
