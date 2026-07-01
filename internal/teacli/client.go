// Package teacli is a thin wrapper around Gitea's official `tea` CLI.
//
// tea-dash does not talk to the Gitea API directly; instead it shells out to
// the user's already-configured `tea` binary. This reuses tea's login
// profiles (~/.config/tea/config.yml), multi-instance support and auth, and
// keeps tea-dash a pure presentation layer.
//
// Two access patterns are provided:
//
//   - API calls `tea api <endpoint>` and returns the raw, fully-typed Gitea
//     REST JSON. This is the preferred path for reading structured data.
//   - Run executes an arbitrary `tea` subcommand (for side effects).
package teacli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultBinary is the executable name looked up on PATH when none is set.
const DefaultBinary = "tea"

// Client runs the `tea` CLI and decodes its output.
type Client struct {
	// Binary is the tea executable to invoke. Defaults to DefaultBinary.
	Binary string
	// Login selects a named tea login profile (tea --login). Optional; when
	// empty, tea uses its default or repository-context-derived login.
	Login string
}

// New returns a Client using the tea binary found on PATH.
func New() *Client {
	return &Client{Binary: DefaultBinary}
}

func (c *Client) binary() string {
	if c.Binary == "" {
		return DefaultBinary
	}
	return c.Binary
}

// API calls `tea api <endpoint>` and, when out is non-nil, decodes the JSON
// response body into it.
func (c *Client) API(ctx context.Context, endpoint string, out any) error {
	stdout, err := c.Run(ctx, "api", endpoint)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(stdout, out); err != nil {
		return fmt.Errorf("teacli: decoding response from %q: %w", endpoint, err)
	}
	return nil
}

// Run executes `tea <args...>` and returns its stdout. A non-zero exit is
// reported as an error carrying tea's stderr (tea writes data to stdout and
// diagnostics to stderr, so the two never mix).
func (c *Client) Run(ctx context.Context, args ...string) ([]byte, error) {
	full := args
	if c.Login != "" {
		full = append([]string{"--login", c.Login}, args...)
	}

	cmd := exec.CommandContext(ctx, c.binary(), full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("teacli: tea %s: %s", strings.Join(full, " "), msg)
		}
		return nil, fmt.Errorf("teacli: tea %s: %w", strings.Join(full, " "), err)
	}
	return stdout.Bytes(), nil
}
