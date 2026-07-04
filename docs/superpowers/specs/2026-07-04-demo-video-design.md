# tea-dash demo video — design

Date: 2026-07-04
Status: approved

## Goal

A short, authentic demo video that gives developers an immediate impression
of the TUI and its capabilities, to drive adoption. Authenticity is a hard
requirement: the video records the real `tea-dash --mock` binary in a real
terminal — never a recreation.

## Approach (chosen: VHS-only)

[charmbracelet/vhs](https://github.com/charmbracelet/vhs) drives the real
binary from a scripted `.tape` file and captures actual rendered frames via a
headless terminal (ttyd + ffmpeg). Deterministic and re-recordable: the tape
is committed, so the demo regenerates in seconds whenever the UI changes.
Rejected: Remotion-composited recreation (violates authenticity; the Remotion
MCP stays available for docs), and live screen-recording a tmux pane
(non-reproducible, needs capture permissions, records operator latency).

## Deliverables

1. `docs/vhs/demo.tape` — the committed recording script (source of truth).
2. `docs/demo.gif` — ~45 s, ~1200 px wide, < 8 MB, embedded as the hero
   image at the top of the README (right after the intro paragraph).
3. `demo.mp4` — same recording, uploaded as a `v0.3.0` release asset and
   linked below the GIF for full quality.

## Script (~45 s, 120×33 terminal, truecolor, default unicode icon theme)

1. Launch `./bin/tea-dash --mock` — framed shell, teahouse PR table, preview.
2. `j`/`k` browse rows — preview follows the selection.
3. `enter` focus the preview, `j`/`d` scroll it, `esc` back.
4. View tour: `2` Issues → `3` Inbox → `4` CI → `5` Branches → `1` Pulls.
5. `:` command palette → type `merge` → run → pick strategy → green success
   toast + the merged PR drops off the list (real mutation — the money shot).
6. `?` help overlay, brief hold, `esc`.
7. `q` quit.

Mouse support (click/wheel/right-click palette) cannot be driven by a VHS
tape; the README caption under the GIF mentions it instead.

## Verification

- Frame inspection: extract frames with ffmpeg and visually review (readable
  text, correct colors/borders, no tearing, pacing).
- Size budget: GIF < 8 MB (re-tune fps/dimensions if over).
- `make check` stays green (docs hygiene).
- User sign-off on the final GIF before the README change ships.

## Implementation plan (inline, single-threaded)

1. `brew install vhs`; `make build`.
2. Write `docs/vhs/demo.tape`; record; iterate on pacing/size via frame
   extraction until the script beats read clearly.
3. Produce GIF + MP4 from the same tape (two Output lines).
4. README: hero GIF + MP4 link + one-line mouse caption; commit; push.
5. `gh release upload v0.3.0 demo.mp4`.
