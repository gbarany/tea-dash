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
