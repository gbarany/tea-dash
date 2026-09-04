<!-- Thanks for sending this. The full guide is in CONTRIBUTING.md.
     Typos, docs and an obvious bug fix: just open the PR. A new key binding, a new
     config field, or a change to what a view shows: please open an issue first.
     Title it like a commit subject, e.g. `fix(ui): keep column widths after a resize`. -->

## What and why

<!-- One or two sentences: what this changes, and the problem it solves. -->

Fixes #

<!-- Fixes / Closes / Resolves #123 closes the issue on merge. Delete the line if there is no issue. -->

## How I tested it

<!-- `make check` is the baseline. Say what you exercised by hand too:
     `go run . --mock` boots the dashboard on built-in demo data, with no Gitea
     server, token or `tea` login needed. -->

## Checklist

- [ ] `make check` passes (gofmt, `go vet`, `go test -race ./...`, public-hygiene)
- [ ] Added a test, or explained above why one does not fit
- [ ] Updated the docs if behaviour changed (README, plus `schema.json` and `examples/tea-dash.yml` for a new config field)

<!-- Optional but appreciated: leave "Allow edits by maintainers" ticked, so small
     fixups do not need a round trip. -->
