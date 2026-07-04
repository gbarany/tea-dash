# tea-dash security scanning & policy — design

Date: 2026-07-04
Status: approved

## Goal

Give evaluators of this public repo visible, verifiable evidence that the
project is maintained responsibly and its releases are trustworthy:
automated scanning surfaced in the GitHub Security tab, a public OpenSSF
Scorecard badge, a disclosure policy, hardened workflows, and cryptographic
build provenance on release binaries (which the Homebrew cask ships
unsigned).

## Deliverables (comprehensive tier, approved)

1. `.github/workflows/codeql.yml` — CodeQL for Go on PRs, pushes to `main`,
   and a weekly cron; `security-events: write` only where needed.
2. `.github/workflows/security.yml` — two jobs on PRs/`main`/weekly:
   `govulncheck ./...` (official Go vulnerability scanner, reachability
   aware) and `gosec` SAST with SARIF upload to the Security tab. Not added
   to required checks initially (owner may promote later).
3. `.github/workflows/scorecard.yml` — OpenSSF Scorecard, weekly + on
   `main`, `publish_results: true`, SARIF upload.
4. `.github/dependabot.yml` — weekly `gomod` and `github-actions` updates.
5. `SECURITY.md` — latest-release support statement, GitHub Private
   Vulnerability Reporting as the disclosure route, response expectations,
   and a "verify a release" section using `gh attestation verify`.
6. Action pinning: every `uses:` in every workflow pinned to a full commit
   SHA with a `# vX.Y.Z` comment (Dependabot keeps them fresh).
7. `release.yml`: `actions/attest-build-provenance` over goreleaser's
   archives + `checksums.txt` (`id-token: write`, `attestations: write`).
   Applies to releases from the next tag onward.
8. README: CI + OpenSSF Scorecard badges under the title; a short Security
   section linking `SECURITY.md`.
9. Repo settings: enable Private Vulnerability Reporting and Dependabot
   alerts (via `gh api`; manual one-clicks documented if the token lacks
   admin scope).

## Non-goals

SLSA L3 builder migration, macOS code signing/notarization, third-party
SaaS scanners, making the new checks required (owner's call later).

## Verification

All changes land through a PR; every new workflow must run green on that PR
(or, for main-only triggers like Scorecard, immediately after merge).
Badge URLs checked reachable after the first Scorecard publish. The
attestation step is exercised by the next release tag, not this PR — its
syntax is validated by workflow parse + a documented dry check.
