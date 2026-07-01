// Package shell builds and runs user shell commands through the user's shell.
package shell

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// Command is an executable shell command plus execution context.
type Command struct {
	Name  string
	Args  []string
	Stdin []byte
	Dir   string
}

// BuildCommand builds a command that runs command through "$SHELL -c", falling
// back to /bin/sh when SHELL is unset. It does not execute the command.
func BuildCommand(command string, stdin []byte, dir string) Command {
	name := os.Getenv("SHELL")
	if name == "" {
		name = "/bin/sh"
	}
	return Command{
		Name:  name,
		Args:  []string{"-c", command},
		Stdin: stdin,
		Dir:   dir,
	}
}

// Runner runs shell commands.
type Runner interface {
	Run(context.Context, Command) ([]byte, error)
}

// ExecRunner runs commands through os/exec.
type ExecRunner struct{}

// Run executes cmd and returns combined stdout/stderr.
func (ExecRunner) Run(ctx context.Context, cmd Command) ([]byte, error) {
	c := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	c.Dir = cmd.Dir
	if cmd.Stdin != nil {
		c.Stdin = bytes.NewReader(cmd.Stdin)
	}
	return c.CombinedOutput()
}
