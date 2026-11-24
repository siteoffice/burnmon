# BurnMon cloud

Counts the tokens of Cowork sessions that run in Anthropic's cloud, for
[BurnMon](https://github.com/siteoffice/burnmon), the local token and cost monitor.

Version 2.0.0 is a mod. It waits on Anthropic: a person's mods do not load in Cowork (Cloud)
sessions yet (measured 2026-11-23), so today it delivers nothing. BurnMon says "cloud mod: no
delivery yet" until the first file lands.

## What it does

- Only inside a Cowork (Cloud) session (`CLAUDE_CODE_ENTRYPOINT` is `remote_cowork`). Anywhere
  else, for example in Claude Code on your laptop, every hook only passes the event on.
- Per model request, main or subagent, it keeps one row: time, session id, request and message
  ids, model, input, output, cache write and cache read tokens. No prompts, no answers, no tool
  input, no paths. The ids are read from the session's own transcript so BurnMon counts a request
  once, also when it reads the same session another way.
- It rewrites `/mnt/user-data/outputs/burnmon-cloud-<session>.jsonl` and, about 10 seconds after
  each turn ends, the Claude app copies it to `~/.burnmon/cloud/mod-<session>.jsonl` on your
  computer (`remote-devices`, `device_commit_files`).
- No network, no telemetry. A failing step never stops your session; the rows then stay in the
  session's outputs folder.

## Set up

1. Install BurnMon. Create the folder `~/.burnmon/cloud` if it is not there.
2. Add this plugin as a personal plugin.
3. Connect the folder `~/.burnmon` to the cloud sessions you want counted. In a Cowork Project,
   add it once to the Project's folders.

## Licence

MIT, see `LICENSE`.
