# CLAUDE.md — tea-dash

A keyboard-driven terminal dashboard for **Gitea/Forgejo**, modeled on `gh-dash` (which is for GitHub).
Go + Bubble Tea v2, talks to Gitea directly via the Go SDK (`code.gitea.io/sdk/gitea`) plus a small raw
HTTP escape hatch. Reuses your `tea` CLI login for auth.

## Build / test / run

```bash
make check      # gofmt-check + go vet + go test -race ./...   (run before every commit)
make build      # -> ./bin/tea-dash (ldflags stamp version/commit/date)
make run        # go run .
```
Tests use the stdlib `testing` package (no testify), with `net/http/httptest` fakes for the Gitea layer.
Requires **Go 1.26+** (the Gitea SDK's go.mod declares `go 1.26`).

## Architecture (orientation)

- `main.go` -> `internal/auth` resolves URL+token, `internal/gitea` builds the client, `internal/ui` runs the TUI.
- `internal/gitea` — SDK client + `search.go` (the me-scoped cross-repo `/repos/issues/search` via a raw
  `rawGet`; **C1 guard**: `@me` filters emit `created=true` etc., never `created_by`). `SearchPulls`/`SearchIssues`.
- `internal/ui` — a `ProgramContext` DI seam (`Icons`, `Styles`) + a `Section` interface with an embeddable
  `BaseModel` (`components/section`, incl. `StateCell`/`GlyphText`); five sections (pull/issue/notification/
  action/branch); `internal/ui/layout` computes the framed-shell rects + mouse zones; `helpoverlay`/`palette`
  read the live `keyMap.Groups`. Root `internal/ui/app.go` holds per-view section slices, routes async results
  by `(SectionId, SectionType)`, and tracks preview-focus/overlay state.
- `internal/config` — yaml.v3; `Instance` (auth), structured `PrIssueFilter`, `prSections`/`issuesSections`, `Defaults`.

## Auth (how the token is resolved)

Order: `instance.token` > `instance.tokenCommand` (stdout) > `instance.tokenEnv` > `TEA_DASH_TOKEN` > the
selected `tea` login's token. tea-dash reads `tea`'s config file (`os.UserConfigDir()/tea/config.yml`); if `tea`
kept the token in the OS **keychain** (empty in the file), use `tokenCommand` with a secret manager:

```yaml
# ~/.config/tea-dash/config.yml
instance:
  tokenCommand: "<command that prints the token>"
```

## Deployment / Release process

Distribution is **Homebrew via a cask tap** (goreleaser). One command cuts a release.

### To ship a new version

```bash
# on main, working tree clean, make check green:
git tag -a vX.Y.Z -m "tea-dash vX.Y.Z — <summary>"
git push origin vX.Y.Z
```

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs `goreleaser release --clean`. GoReleaser:
1. Builds static binaries (`CGO_ENABLED=0`) for linux/darwin/windows × amd64/arm64 + `checksums.txt`.
2. Creates the GitHub Release with those archives.
3. Generates `Casks/tea-dash.rb` and **pushes it to `gbarany/homebrew-tap`** (config: `homebrew_casks` in
   `.goreleaser.yaml`). Users then `brew install gbarany/tap/tea-dash` (macOS + Linux).

### Why `homebrew_casks`, not `brews`

GoReleaser deprecated `brews`/formula for pre-compiled binaries (fully deprecated in v2.16). Casks are the
right tool and now work on Linux too (Homebrew brew#19121). The cask has a macOS post-install hook that strips
`com.apple.quarantine` (the binaries are **unsigned/unnotarized**), else Gatekeeper reports "tea-dash is damaged".

### One-time setup (already done; documented for reference)

- `gbarany/tea-dash` is **public** (release assets must be publicly downloadable for `brew install`).
- `gbarany/homebrew-tap` repo exists (public, `main`).
- Repo secret **`HOMEBREW_TAP_GITHUB_TOKEN`** = a fine-grained PAT with **Contents: read/write on
  `gbarany/homebrew-tap`** (the default Actions `GITHUB_TOKEN` can only write the current repo, not the tap).
  Passed to goreleaser in `release.yml`.

### Local validation before tagging (optional)

```bash
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser check                 # validate .goreleaser.yaml
goreleaser release --snapshot --clean --skip=publish   # dry-run a full build locally
```

### Install (end users)

```bash
brew install gbarany/tap/tea-dash     # macOS + Linux
```
Do not commit private machine paths, secret-manager item routes, personal access tokens, or tenant-specific examples.
Use placeholders such as `<repo-path>` and `<command that prints the token>` in committed docs and tests.

### homebrew-core (later, optional)

`brew install tea-dash` (no tap prefix) means getting into Homebrew/homebrew-core — needs notability criteria +
a PR to homebrew-core. The tap is sufficient for now.
