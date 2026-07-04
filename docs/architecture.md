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

Most reads go through the typed SDK. The exception is the **me-scoped,
cross-repo pull-request/issue search**, whose `created=true` / `assigned=true`
booleans are not expressible through the SDK's per-repo option structs. That
path is served by a small raw HTTP escape hatch (`rawGet`) calling
`GET /repos/issues/search?type=pulls&created=true&state=…` and tolerantly
decoding rows (unknown fields ignored). When a section declares
`repo: owner/name`, tea-dash uses the typed repo issues endpoint instead, so
plain login filters such as `createdBy: alice` can be honored. When global
`repos:` is configured and a PR/issue section omits `repo:`, tea-dash fans out
to that same repo-scoped endpoint for every listed repo, merges rows by updated
time, and slices the requested global page for progressive loading. PR sections
with `reviewRequested` stay on the cross-repo search endpoint because Gitea does
not expose that filter on the repo issues endpoint. Results are mapped into the
domain model, never leaking SDK/REST types past the transport boundary.

`internal/mockgitea` fakes the subset of this API the client consumes: an
in-memory, mutation-capable store served over a local HTTP listener, used by
`--mock` (with a deterministic `teahouse` demo dataset) and by the end-to-end
tests. Its contract tests drive the real `internal/gitea` client against the
fake, so response-shape drift fails tests instead of surfacing at runtime;
unknown paths 404 with the offending method+path in the body for the same
reason.

## Domain model

`internal/data` holds TUI-agnostic domain types (notably `PullRequest`),
decoupled from both the Gitea transport and the Bubble Tea UI. A `PullRequest`
is denormalized — each row from the cross-repo search carries its own
`owner/repo` — so the UI can render a flat table without extra lookups.

## Package layout

| Package              | Responsibility                                                                                                  |
| -------------------- | --------------------------------------------------------------------------------------------------------------- |
| `main`               | entrypoint, `--version`/`--help`; loads config, resolves auth, builds the client, starts the Bubble Tea program |
| `internal/ui`        | root model, table, keybindings, Lipgloss styles, loading/error states                                           |
| `internal/gitea`     | Gitea SDK wrapper + raw HTTP escape hatch (me-scoped cross-repo search)                                         |
| `internal/auth`      | resolves the instance URL + token from overrides, env, and `tea`'s config                                       |
| `internal/data`      | TUI-agnostic domain model (`PullRequest`, `Label`, …)                                                           |
| `internal/config`    | loads `~/.config/tea-dash/config.yml` (instance block, repos)                                                   |
| `internal/mockgitea` | in-memory fake Gitea (HTTP) behind `--mock` and the end-to-end tests                                            |
| `internal/build`     | version metadata injected at link time via `-ldflags`                                                           |

## Current feature surface

- Configurable PR/issue sections with structured filters, repo fan-out,
  current-repo smart filtering, progressive loading, and configurable table
  columns.
- Default-open preview panes with markdown bodies, comments, review summaries,
  CI/check status, and Actions run detail.
- Mutations for common PR, issue, notification, Actions, and local branch
  workflows: comments, labels, assignment, milestones, subscriptions,
  close/reopen, reviews, reviewer requests, merge variants, branch checkout,
  branch push/delete, notification read/pin state, and Actions logs/rerun/cancel.
- Keyboard and mouse interaction shaped after `gh-dash` and `lazygit`, plus
  configurable built-in/custom keybindings.
- Public distribution through GitHub releases and the Homebrew tap.

## Future refinements

- Deeper theme coverage beyond the current core text/background/border palette.
- More capability probes for server-version differences that emerge in the
  Forgejo/Gitea Actions and review APIs.
- Optional multi-instance switching inside one running session; today a session
  connects to one resolved instance.
