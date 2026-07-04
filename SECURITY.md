# Security Policy

## Supported versions

Only the **latest release** receives security fixes. Update with
`brew upgrade --cask gbarany/tap/tea-dash` (or reinstall via `go install`).

## Reporting a vulnerability

Please use **GitHub Private Vulnerability Reporting**:
[Security → Report a vulnerability](https://github.com/gbarany/tea-dash/security/advisories/new).
Do not open public issues for suspected vulnerabilities.

You can expect an acknowledgement within a week. This is a spare-time
project, so fix timelines vary with severity — critical issues (credential
exposure, remote code execution paths) take priority over everything else.

## What tea-dash does with your credentials

tea-dash reads the Gitea token from your `tea` CLI config (or from
`token`/`tokenCommand`/`tokenEnv` in its own config) and sends it only to
your configured Gitea/Forgejo instance over HTTPS. It never writes tokens to
disk, logs, or any third party. `tea-dash --mock` makes no network
connections at all.

## Automated scanning

Every pull request and a weekly schedule run:

- **CodeQL** (code scanning, results in the Security tab)
- **govulncheck** (Go vulnerability database, reachability-aware)
- **gosec** (Go SAST, SARIF results in the Security tab)
- **OpenSSF Scorecard** (repository security practices; public report via
  the README badge)

Dependabot keeps Go modules and pinned GitHub Actions current.

## Verifying a release

Release archives built from `v0.3.1` onward carry GitHub build provenance
attestations. To verify an archive was built by this repository's release
workflow from a tagged commit:

```sh
gh attestation verify tea-dash_<version>_<os>_<arch>.tar.gz --repo gbarany/tea-dash
```

`checksums.txt` in each release covers all archives; the Homebrew cask
verifies its own SHA256 automatically.
