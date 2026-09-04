# Contributing to tea-dash

**Contributions are welcome — pull requests included.** If you use tea-dash and
something is broken, missing, or awkward, I'd rather hear about it than not.

tea-dash is maintained by one person in their spare time, so this guide exists
to make your contribution land smoothly: what to send, how to run the thing
locally, and the handful of invariants that will bite you if nobody mentions
them.

## Ways to contribute

- **Bug reports.** The most valuable thing you can send. Include your OS,
  terminal, and Gitea/Forgejo version — and, this one really helps, whether the
  bug also reproduces under `tea-dash --mock`. If it does, I can fix it without
  access to your server.
- **Feature ideas.** Describe the workflow you're trying to get through, not
  just the feature. tea-dash is opinionated, and the "why" is what decides
  whether something fits.
- **Documentation.** README wording, config examples, the architecture doc.
  Doc-only PRs need no issue and no ceremony.
- **Code.** See below.

Everything goes through [GitHub issues](https://github.com/gbarany/tea-dash/issues).

### Where to start

- [`good first issue`](https://github.com/gbarany/tea-dash/labels/good%20first%20issue)
  — small, self-contained, and described well enough to pick up cold.
- [`help wanted`](https://github.com/gbarany/tea-dash/labels/help%20wanted)
  — things I'd genuinely like a hand with.

If both lists are empty, any open issue is fair game — comment on it first so we
don't both write the same patch.

## Before you start

**Just open the PR** — no issue needed — for:

- typos, wording, and documentation fixes;
- an obvious bug fix that comes with a regression test;
- dependency and CI maintenance.

**Open an issue first** for anything that changes the shape of the app:

- a new key binding, or a change to an existing one;
- a new config field — `internal/config/config.go`, `examples/tea-dash.yml` and
  `schema.json` all move together (plus the README's annotated config block,
  which nothing enforces), so the name is worth agreeing on before you spend an
  evening on it;
- changing what a view shows, or adding a view or section type;
- anything that changes how the Gitea API is called.

This isn't bureaucracy. It's so we agree on the design before you build it, and
so I never have to turn down finished work.

## Development setup

You need **Go 1.26+** (matching `go.mod` and the Gitea SDK's own `go 1.26`) and
`git` on your `PATH`. You do **not** need a Gitea server, a token, or the `tea`
CLI to work on tea-dash.

```sh
git clone https://github.com/gbarany/tea-dash
cd tea-dash

make run        # go run .
make build      # -> ./bin/tea-dash
make check      # run this before you push (see below)
make help       # list every target
```

Run the whole dashboard with no server, no login, and no network:

```sh
go run . --mock
```

`--mock` boots an in-process fake Gitea preloaded with a fictional `teahouse`
org. Every view is populated and actions really mutate the demo data. It also
seeds a throwaway local git repo for the Branches view, which is why `git` is
required. Caveats are listed under
[Try it without a Gitea instance](README.md#try-it-without-a-gitea-instance).

### The one command

```sh
make check      # gofmt-check + go vet + go test -race ./... + public-hygiene
```

CI runs the same four gates plus `go mod download` and `go build ./...`, so
**a green `make check && go build ./...` locally means a green `ci` run.**

While you work:

```sh
go test ./internal/gitea/                        # one package
go test -run TestSearchPulls ./internal/gitea/   # one test (-run takes a regex)
go test -race ./...                              # what CI runs
```

`make lint` runs `golangci-lint` if you have v2+ installed. It's optional, and
part of neither `make check` nor CI.

## Project layout

The README's [Development](README.md#development) section has the package layout
block — start there for *where* things live.
[`docs/architecture.md`](docs/architecture.md) explains *why*: the SDK-direct
transport, auth resolution, and the domain-model boundary. The specs and plans
under `docs/superpowers/` are the running record of how features got built; the
habit here is to land a `docs:` spec commit before a large feature.

Two entry points you'll most likely want:

- **A new UI section** is thin wiring over the generic `section.Model[T]`. Use
  `internal/ui/components/pullsection/pullsection.go` as the template, then wire
  it into `buildSections` in `internal/ui/app.go`.
- **A new Gitea call** goes in `internal/gitea/<domain>.go` using the typed SDK,
  mapped into `internal/data` before it reaches the UI.

## House rules that will trip you up

None of these are style preferences, and most have a test or a script behind
them.

- **Never commit an absolute home-directory path, a macOS per-user temporary
  path, or a secret-manager item route.** `scripts/check-public-hygiene.sh` (run
  by `make check`) `git grep`s the whole tree for them and fails the build. This
  is a public repo and those strings leak a maintainer's machine layout. Use
  placeholders like `<repo-path>` and `<command that prints the token>` in
  docs, tests, and comments. It uses `git grep`, so it only sees *tracked* files
  — `git add` a new file before trusting a green run.
- **Tests use the standard library `testing` package only.** No testify — it
  shows up in `go.sum` only, via a dependency's own go.mod, and is imported by
  zero files here. Gitea-layer tests use `net/http/httptest` fakes; see `fakeGitea` in
  `internal/gitea/client_test.go`, which always stubs `/api/v1/version` and
  `/api/v1/user` because the SDK probes both at construction.
- **Current practice: no `testdata/` directory, no golden files, no
  `t.Parallel()`.** Fixtures are
  Go string constants next to the test. The end-to-end harness is hand-rolled in
  `internal/ui/e2e_mock_test.go` (`newE2EModel`, `drain`) — `teatest` is not used.
- **The C1 guard.** On the cross-repo `/repos/issues/search` endpoint, an `@me`
  filter must be emitted as that endpoint's boolean flag (`created=true`,
  `assigned=true`, `mentioned=true`, `review_requested=true`), never as the
  per-repo `created_by`/`assigned_by` params, which that endpoint silently
  ignores. It's documented on `buildSearchParams` in `internal/gitea/search.go`
  and enforced by `internal/gitea/search_test.go`. Corollary: `config.Validate`
  rejects a non-`@me` author filter on any section that isn't repo-scoped.
- **A new Gitea endpoint needs a matching `internal/mockgitea` route**, or
  `--mock` and every e2e test start 404ing with
  `mockgitea: no handler for <METHOD> <PATH>`. Two landmines inside a handler:
  the store mutex is non-reentrant (inside `store.WithLock` use only the
  unexported `*Locked` accessors — an exported getter self-locks and deadlocks),
  and picking the right response helper matters.
  `internal/mockgitea/server.go` documents both.
- **Two silent-failure traps in `section.Options`:** `Options.Type` must
  correspond to the type parameter `T`, because it drives app-level fetch
  routing and a mismatch misroutes results with no error; and `section.New`
  panics if `Fetch`, `BuildRow` or `Limit` is nil. `T` must satisfy
  `data.RowData`.
- **Config changes move three files together:** `internal/config/config.go`,
  `examples/tea-dash.yml`, and `schema.json`. The schema uses
  `additionalProperties: false` throughout and `schema_test.go` validates the
  example against it, so forgetting the schema fails `go test ./...` at the repo
  root in a way that looks unrelated.
- **`internal/ui/context` shadows the stdlib `context` package.** House
  convention is to alias: `stdctx "context"` and `appctx ".../ui/context"`.
  Components read glyphs and colors from `ctx.Icons` / `ctx.Styles` rather than
  hardcoding them.
- **This is Bubble Tea v2.** Imports are `charm.land/bubbletea/v2` and friends,
  not `github.com/charmbracelet/*`, and `View()` returns a struct — read
  `.Content`. v1 habits will not compile.
- **Third-party GitHub Actions are SHA-pinned** with a `# vX.Y.Z` trailing
  comment; keep that form when bumping one, and move the `github/codeql-action`
  steps in lockstep.

## Submitting a PR

1. Branch off `main` in your fork.
2. Keep it focused — one concern per PR. A 400-line PR fixing three unrelated
   things takes far longer to review than three small ones.
3. Include a test. A bug fix should come with the regression test that fails
   without it.
4. `make check` must pass, and so must `go build ./...`.
5. Leave **"Allow edits by maintainers"** ticked (it's on by default). That's
   your checkbox, not something I can enable — with it on I can push a small
   fix-up instead of sending the PR back over a nit.
6. Say in the description if the change was AI-assisted (see below).

### Commit messages

The history is [Conventional Commits](https://www.conventionalcommits.org):

```
feat: interactive label filter for PR and issue lists
fix(sidebar): keep selected preview tab across re-render (sticky by title)
```

Types in use: `feat`, `fix`, `docs`, `chore`, `test`, `refactor`, `ci`, `build`.
Scopes seen in the history: `ui`, `plan`, `mockgitea`, `deps`, `config`, `prs`,
`actions`, `security`, `keys`, `cli`, `branches` — plus one-offs. Pick the
closest one or omit it. Add `!` before the colon for a breaking change.

This is load-bearing rather than decorative: `.goreleaser.yaml`'s changelog
filters drop `docs:`, `test:`, `ci:` and `chore:` commits, so everything else —
`feat:`, `fix:`, `refactor:`, `build:` — shows up in the release notes. If a
prefix isn't quite right I'd rather fix it at merge time than send the PR back
for it.

### What CI does

Three workflows run on every pull request:

- **ci** — `go mod download`, gofmt, `go vet ./...`, `go build ./...`,
  `go test -race ./...`, and the public-hygiene script, on `ubuntu-latest` with
  Go `stable`. This is the one that has to be green.
- **CodeQL** — code scanning; results land in the Security tab.
- **security** — `govulncheck` (reachability-aware, and it *can* fail your PR)
  and `gosec` (runs with `-no-fail`, so its findings are advisory SARIF only).

On your first PR from a fork the checks sit at **"Awaiting approval"** until I
click "Approve workflows to run". That's GitHub's default for first-time
contributors, not me ignoring you.

### AI-assisted contributions

Fine by me — I use these tools on this repo myself — with three conditions:

1. Say so in the PR description.
2. You understand the change well enough to explain and defend it. If you can't
   say what it does and how it interacts with the rest of the app, don't send it.
3. Review questions are for you, not for a model. If I ask why something works
   the way it does, I need your answer.

Large generated PRs that arrive out of the blue get closed. That's about review
time, not about the tool.

### Being decent to each other

This project has a [Code of Conduct](CODE_OF_CONDUCT.md) — be kind, assume good
faith, and help people who are learning. Harassment isn't tolerated and I'll act
on it.

## Review expectations

This is a spare-time project, so here's the honest version:

- I'll acknowledge issues and PRs **within about a week** — the same promise
  [SECURITY.md](SECURITY.md) makes. A full review can take longer.
- Rough priority: bugs and crashes first, then small features, then refactors.
  A refactor with no user-visible benefit is a hard sell.
- **Heard nothing in two weeks? Ping the thread.** That's helpful, not rude — it
  means I dropped it, not that I'm ignoring you.
- I may decline a good feature simply because I don't want to maintain it
  forever. That's not a judgment on your code, and I'll try to say so before you
  build it rather than after — which is the whole point of the "open an issue
  first" list above.

Please also be willing to iterate on review comments. A drive-by PR that solves
your own problem and is then abandoned costs more than it saves — keeping it in
your fork and linking it from an issue is a perfectly good outcome.

## License

tea-dash is [MIT licensed](LICENSE), and contributions are accepted under that
same license. By opening a pull request you affirm that you wrote the
contribution or otherwise have the right to submit it, and that it may be
provided under the project's MIT license.

That's all of it: no CLA, no copyright assignment, nothing to sign. `git commit -s`
sign-offs are welcome if your employer wants them, but they are not required here.
