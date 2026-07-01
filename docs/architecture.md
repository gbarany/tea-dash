# Architecture

tea-dash is a Bubble Tea TUI that talks to Gitea **directly**, using Gitea's
official Go SDK ([`code.gitea.io/sdk/gitea`](https://pkg.go.dev/code.gitea.io/sdk/gitea)).
It does **not** shell out to the `tea` CLI at runtime. It does, however, reuse
the login profiles `tea` already stores on disk, so you get auth and
multi-instance support without tea-dash managing credentials of its own.

## Why the SDK (and not the `tea` CLI)

- **Typed, complete data.** The SDK returns real Gitea REST structs, so the data
  layer works with typed values instead of parsing stringly-typed CLI output.
- **No subprocess at runtime.** `tea` does not need to be on `PATH` when tea-dash
  runs. It is only needed **once**, to create a login (`tea login add`).
- **Auth & multi-instance for (almost) free.** `tea` stores named login profiles
  in its config file (`~/Library/Application Support/tea/config.yml` on macOS,
  `~/.config/tea/config.yml` on Linux). tea-dash reads that file to recover the
  instance URL and token instead of re-implementing token/OAuth/SSH management.
- **Broad compatibility.** Anything the SDK can reach (Gitea, Forgejo, Codeberg)
  tea-dash can display.

## Auth resolution

`internal/auth` resolves a `Config{URL, Token, Insecure, CACertPath}` from three
sources, in precedence order:

1. **Explicit overrides** from tea-dash's own `instance:` block in
   `~/.config/tea-dash/config.yml` (URL, token, TLS options, and the name of the
   `tea` login to select).
2. **Environment**: `TEA_DASH_URL` / `TEA_DASH_TOKEN`.
3. **The `tea` login** picked from `tea`'s config file — by name if the config
   selects one, else the login marked `default`, else the sole login.

A missing `tea` config is not an error: overrides or env vars may fully specify
the connection. If neither a URL nor a token can be resolved, startup fails with
an actionable message.

## Gitea transport

`internal/gitea` wraps the SDK client. `NewClient`:

- negotiates TLS (honouring `insecureSkipVerify` / a private `caCert` bundle),
- pins a single shared `*http.Client` (30s timeout) used by both the SDK and the
  raw escape hatch,
- caches the authenticated user's login via `GetMyUserInfo` (exposed as `Me()`).

Most reads go through the typed SDK. The one exception is the **me-scoped,
cross-repo pull-request search**, which the typed SDK cannot express: it is
served by a small raw HTTP escape hatch (`rawGet`) that calls
`GET /repos/issues/search?type=pulls&created=true&state=…` and tolerantly
decodes the rows (unknown fields ignored). Results are mapped into the domain
model, never leaking SDK/REST types past the transport boundary.

## Domain model

`internal/data` holds TUI-agnostic domain types (notably `PullRequest`),
decoupled from both the Gitea transport and the Bubble Tea UI. A `PullRequest`
is denormalized — each row from the cross-repo search carries its own
`owner/repo` — so the UI can render a flat table without extra lookups.

## Package layout

| Package           | Responsibility                                                            |
| ----------------- | ------------------------------------------------------------------------- |
| `main`            | entrypoint, `--version`/`--help`; loads config, resolves auth, builds the client, starts the Bubble Tea program |
| `internal/ui`     | root model, table, keybindings, Lipgloss styles, loading/error states     |
| `internal/gitea`  | Gitea SDK wrapper + raw HTTP escape hatch (me-scoped cross-repo PR search) |
| `internal/auth`   | resolves the instance URL + token from overrides, env, and `tea`'s config |
| `internal/data`   | TUI-agnostic domain model (`PullRequest`, `Label`, …)                     |
| `internal/config` | loads `~/.config/tea-dash/config.yml` (instance block, repos)             |
| `internal/build`  | version metadata injected at link time via `-ldflags`                     |

## Roadmap (next steps)

1. Add config-driven, per-repo sections and per-section filters (à la `gh-dash`).
2. Issue and notification sections.
3. A detail pane (PR/issue body via the SDK, rendered with `glamour`).
4. Actions (checkout, merge, comment, close, …) through the SDK.
