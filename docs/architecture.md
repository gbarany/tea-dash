# Architecture

tea-dash is a Bubble Tea TUI that renders data obtained by shelling out to
Gitea's official [`tea`](https://gitea.com/gitea/tea) CLI. It deliberately does
**not** implement its own Gitea API client or credential storage.

## Why shell out to `tea`

- **Auth & multi-instance for free.** `tea` stores named login profiles in
  `~/.config/tea/config.yml` and resolves the active login from repo context or
  the `--login` flag. tea-dash reuses this instead of re-implementing tokens,
  SSH, OAuth and instance management.
- **Pure presentation layer.** No secrets pass through tea-dash.
- **Broad compatibility.** Anything `tea` can reach (Gitea, Forgejo, Codeberg),
  tea-dash can display.

## The three output tiers of `tea`

Understanding how `tea` emits machine-readable data drives the data layer:

1. **List commands** — `tea <entity> list -o json`. Emits an array of **flat
   objects whose keys are the selected `--fields` (snake_cased) and whose values
   are all strings** (numbers, booleans and dates included). You choose the keys
   via `--fields`. Cheap, but lossy/untyped.
2. **Single-item detail** — `tea issues <n> -o json` / `tea pulls <n> -o json`.
   Emits a **curated, properly-typed struct** with a fixed schema (comments only
   when `--comments` is passed).
3. **`tea api <endpoint>`** — streams the **raw, complete, typed Gitea REST
   JSON**, supports query params and server pagination. This is the richest and
   most predictable source.

**Decision:** tea-dash reads primarily via **`tea api`** (tier 3), decoding into
Go structs in `internal/teacli`. Tier-1 lists are used only where a flat string
table is sufficient. Mutations (checkout, merge, comment, close, …) go through
the porcelain subcommands with explicit flags.

## Gotchas the data layer must respect

- **Never request the PR `ci` field in a bulk list**: it triggers an N+1 status
  fetch and, on error, writes to **stdout**, corrupting JSON. Fetch CI status
  lazily/separately.
- **Always pass `-o json` on detail views**: without it, `tea` renders markdown
  and may prompt interactively for comments.
- **Avoid interactive paths**: `tea login add` (no flags), `tea pulls review`,
  and any body/`$EDITOR` prompt. Always pass bodies and `--confirm`/`-y`.
- **stdout = data, stderr = errors**; non-zero exit signals failure. The
  `teacli.Client` surfaces stderr in returned errors.
- **PR list filtering is weak** in `tea pulls list` (state only). For rich PR
  filters use `tea issues list --kind pulls …` or `tea api …/pulls?…`.

## Package layout

| Package            | Responsibility                                            |
| ------------------ | --------------------------------------------------------- |
| `main`             | entrypoint, `--version`/`--help`, starts the Bubble Tea program |
| `internal/ui`      | root model, sections/tabs, keybindings, Lipgloss styles   |
| `internal/teacli`  | wrapper around the `tea` binary (`Run`, `API`) + types    |
| `internal/build`   | version metadata injected at link time via `-ldflags`     |

## Roadmap (next steps)

1. Wire the Pull Requests section to `teacli.ListRepoPulls` with a Bubbles
   `table`/`list` and a loading/error state.
2. Add config (repos/instances to watch, per-section filters) à la `gh-dash`.
3. Issue and notification sections.
4. Detail pane (tier-2/`tea api`) and actions (checkout, merge, comment).
