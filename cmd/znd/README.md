# znd

One program for Windows and Mac. It answers a Foreman spec gate, runs a git recipe,
updates mod folders and writes audit lines. It makes no network call and starts no shell.

Every acting command takes exactly one of `--dry-run` (print, change nothing) or
`--yes` (act). A button press equals `--yes`. Every act writes a start line and a result
line to `controls-audit.ndjson` next to the BurnMon store file. If the start line
cannot be written, nothing runs.

## Commands

    znd answer <gate-wait-id> approve|send-back (--dry-run | --yes)
    znd git save|share|land|tag|sync --cwd <repo> [recipe flags] (--dry-run | --yes)
    znd update-mods --from <folder> --to <mod folder> (--dry-run | --yes)
    znd audit --control <name> --target <text> --exit <code>
    znd --version

- `answer`: spec gates only. The accept gate stays with the owner.
- `git`: the five recipes of snip.py v0.7.0. Flags: `--message`, `--body`, `--co-author`,
  `--files <f>...`, `--tag`, `--subject`, `--expect`, `--runs-in`, `--needs`.
- `update-mods`: `--from` is a local folder holding `herdr-app\mods.json` (mod names and
  a SHA-256 per file) and one folder per mod. `--to` is the mod folder of the app.
  A file you edited moves to `znd-kept\<UTC stamp>\<mod>\`. The old folder moves to
  `znd-old\<UTC stamp>\<mod>\`. Both sit next to `--to`. Nothing is deleted. The hashes
  installed last time sit in `znd-installed.json`, also next to `--to`.
- `audit`: control name `[a-z_]{1,32}`, target cut to 200 characters, no line break.
  It runs nothing, so it has no dry run.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | done, or the dry run printed |
| 1 | a step failed (git line, update step, audit write) |
| 2 | usage error, bad `znd.json`, or a recipe that refuses its input |
| 3 | `answer`: the gate changed or was already decided |
| 4 | `answer`: the verdict is not valid |
| 5 | refused: a precondition is not met, with the reason on stderr |

## What it needs

In `burnmon.json`: `foreman_root` for `answer`, `groundwork_engine` (full path of
`git_flow.py`) for `git land`. `answer` also needs python on PATH (`python3` first on
a Mac) and BurnMon or BurnMon Dev open, unless `znd.json` says otherwise.

`znd.json`, next to the `znd` exe or in the BurnMon data folder:

    { "gate_without_snapshot": "refuse" }

- `refuse` (default): with no current snapshot, `answer` refuses.
- `foreman_read`: with no current snapshot, `answer` decides from a fresh Foreman read.
  A current snapshot is never overruled. Audit lines carry `"via":"foreman_read"`.
- Any other value: exit 2.
