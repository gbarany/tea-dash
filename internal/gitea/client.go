// Package gitea is tea-dash's Gitea transport: a thin wrapper over the
// code.gitea.io/sdk/gitea SDK plus a raw HTTP escape hatch for endpoints the
// typed SDK cannot express (notably the me-scoped cross-repo issue search).
package gitea

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/auth"
)

// Client wraps the SDK client with the resolved identity and a shared HTTP
// client used both by the SDK and by the raw escape hatch.
type Client struct {
	sdk        *sdk.Client
	baseURL    string
	token      string
	httpClient *http.Client
	me         string

	// sdkMu serializes typed SDK calls through one wrapper point. The SDK itself
	// is concurrency-safe; keeping this narrow gate makes future per-call
	// context/cancellation work explicit and keeps detail fetches conservative.
	sdkMu sync.Mutex
}

// call runs fn with the SDK mutex held, serializing typed SDK calls through a
// single wrapper point.
func (c *Client) call(fn func() error) error {
	c.sdkMu.Lock()
	defer c.sdkMu.Unlock()
	return fn()
}

// NewClient builds a Gitea client from resolved auth, negotiating TLS,
// pinning the shared HTTP client, and caching the current user's login.
func NewClient(ctx context.Context, cfg auth.Config) (*Client, error) {
	tlsCfg := &tls.Config{}
	switch {
	case cfg.Insecure:
		tlsCfg.InsecureSkipVerify = true
	case cfg.CACertPath != "":
		pem, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert %s: %w", cfg.CACertPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", cfg.CACertPath)
		}
		tlsCfg.RootCAs = pool
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	hc := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}

	client, err := sdk.NewClient(cfg.URL,
		sdk.SetToken(cfg.Token),
		sdk.SetContext(ctx),
		sdk.SetHTTPClient(hc),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing Gitea client: %w", err)
	}

	me, _, err := client.GetMyUserInfo()
	if err != nil {
		return nil, fmt.Errorf("resolving current user: %w", err)
	}

	return &Client{
		sdk:        client,
		baseURL:    cfg.URL,
		token:      cfg.Token,
		httpClient: hc,
		me:         me.UserName,
	}, nil
}

// Me returns the authenticated user's login.
func (c *Client) Me() string { return c.me }
