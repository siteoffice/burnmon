---
type: plan
title: Cowork (Cloud) mod route, plan and lane spec (burnmon-cloud 2.0.0)
created: 2026-11-24
tags: [burnmon, cowork-cloud, mods, grill-me-product]
status: approved by Wilco 2026-11-24; nothing built
from: Claude (Opus 5.5), grill-me-product phase 6; note `04_assets\2026-11-23_cloud_usage_route_architecture.md` (decided)
---

# Cowork (Cloud) mod route: plan and lane spec

## 0. TL;DR

Build `burnmon-cloud` 2.0.0 now, as Wilco decided on 2026-11-23: a mod that, only inside a Cowork (Cloud) session,
writes one usage row per model request and copies the file to `~\.burnmon\cloud` on the linked computer. Today it
will deliver nothing, because a person's mods do not load in cloud sessions (measured 2026-11-23). BurnMon reads the
files with its existing cloud reader and says "cloud mod: no delivery yet" until the first file lands. One lane,
after the honesty lane lands. The first real test happens the day Anthropic opens mods in cloud sessions.

## 1. Hypotheses

| # | Hypothesis | Measure | Kill signal |
|---|---|---|---|
| H1 | Once mods load in Cowork (Cloud), every main and subagent request of a session reaches `~\.burnmon\cloud` | Rows per session against the session's own turn count; subagent rows present | Mods load but `device_commit_files` from a mod is refused or prompts every turn |
| H2 | A session read both by the mod and by the teleport pull is counted once | Store test with the same request in both sources; one live session read both ways | The mod's rows carry no id the pulled transcript also carries |
| H3 | Outside a Cowork (Cloud) session the mod does nothing | Debug log of a local Claude Code session with the mod installed: no `$.fs.write (burnmon-cloud)`, no `$.mcp.call` | Any write, call or prompt outside a cloud session |

## 2. Scope

In:
- `plugins\burnmon-cloud\` in the burnmon repo: `.claude-plugin\plugin.json` (name `burnmon-cloud`, version 2.0.0),
  `hooks\hooks.json` (`{"modules": ["./register.js"]}`), `hooks\register.js`, `README.md`, `LICENSE`.
- The mod: acts only when `$.env.get('CLAUDE_CODE_ENTRYPOINT')` is `remote_cowork` (measured in this cloud session,
  2026-11-23). `turn.step`: one row per request, the fields of today's drop format (`internal\adapter\claude\cloud.go`,
  `cloudRow`): source `burnmon-cloud-mod`, ts, session_id, request_id, message_id, model, agent (`main` or
  `agent-<id>` from `e.agentId`), input, output, cache creation and cache read tokens. No text, no tool input, no
  paths. File: `/mnt/user-data/outputs/burnmon-cloud-<session>.jsonl`, rewritten whole (under 4 MiB per file).
  `turn.complete` of the main agent: `$.mcp.call('remote-devices', 'device_commit_files', ...)` to
  `~/.burnmon/cloud/mod-<session>.jsonl` with force. Every hook passes the event on and fails open (`.catch`).
- BurnMon: the scan of `~\.burnmon\cloud` reads `mod-*.jsonl` with the existing parser; a de-duplication rule so a
  request stored from a pulled transcript and from a mod row counts once (exact key set by lane step 1); the reason
  line under the strip adds "cloud mod: no delivery yet" or "cloud mod: newest delivery <local date time>", from the
  newest `mod-*.jsonl` (decision 3). Placement (Wilco, 2026-11-24): "no delivery yet" is appended only while the
  reason line already shows (pull partial or failed); "newest delivery" shows on its own too, so the first delivery
  is seen with the pull on, off or working.
- README: "Cowork in the cloud" rewritten (the plugin returns as a mod, what it needs, that it waits on Anthropic);
  `_archive\burnmon-cloud` keeps 1.1.4 with one line pointing to 2.0.0.

Out: any network or port; telemetry (a person's mod gets none, measured); Cowork sessions started on the phone or
the web; publishing the plugin anywhere (Wilco's step); the organization's plugin page; the teleport pull's logic.

## 3. Roles

- Wilco: approves this spec; lands; uploads the built `.plugin` as a personal plugin; publishes it, if ever.
- Claude Code session on the laptop: the lane, tests first, siteoffice-git, stops before land.
- Claude (this Project): the grill state and the hub brief.

## 4. Timeline

| When | What | Done when |
|---|---|---|
| After the honesty lane lands | Lane step 1: measure on the laptop which ids a `turn.step` result carries (`claude -p --plugin-dir` with `CLAUDE_CODE_ENTRYPOINT=remote_cowork` set for that run) | The de-duplication key is written into this spec |
| Same lane | Tests, then the mod, the reader rule, the reason line, README | `go vet`, `go test`, `.\build.ps1`, `uicheck` for the strip, `claude plugin validate` pass |
| After land | Wilco uploads the `.plugin` as a personal plugin | Settings shows burnmon-cloud 2.0.0 under "Created by you" |
| Open-ended | The first Cowork (Cloud) session with mods loaded | A `mod-*.jsonl` lands; H1 to H3 graded |

Caveat: the open-ended row depends on Anthropic. Nothing in this plan can bring it forward.

### 4a. Lane step 1 result: the de-duplication key (measured 2026-11-24)

Measured on the laptop, Claude Code 2.1.331, with a throwaway probe mod hot-loaded in an interactive session.
`claude -p --plugin-dir` loads no mods on this laptop today: every mod logs "not loaded: this session has not read
your organization's managed settings" (debug log, 2026-11-24 00:36 UTC), so the probe ran interactively instead.

- `turn.step` carries no request or message id. Its input has `turnId`, `index`, `model`, `effort`, `messageCount`
  and `agentId`; its result has `turnId`, `index`, `answer`, `toolUses`, `stopReason`, `usage`.
- Once `next(e)` settles, `$.session.messages({ last: 2 })` (for a subagent `{ agentId, last: 2 }`) returns the
  step's own transcript row as the newest assistant message, with its `uuid`.
- `$.session.info().transcriptPath` names the transcript; a subagent's is `<transcript without .jsonl>/subagents/
  agent-<agentId>.jsonl`. The row with that `uuid` carries `requestId` and `message.id`.
- Result: 11 of 11 main-loop steps and 2 of 2 subagent steps resolved to a distinct `requestId`, each with the same
  output tokens as the step's `usage`. Subagent rows are on disk when the step settles; main-loop rows are written
  later, and the turn's final step was not on disk even at `turn.complete` (found on the next read).

**Key:** a mod row's `request_id` is the transcript's `requestId` for the step's newest assistant `uuid`, and
`message_id` its `message.id`. That is the key the Claude parser and the pull already store, so the existing store
rule (a pulled `session_...` row is the one copy of its request id, other copies are removed or skipped) counts the
request once; no new store rule. A row not yet resolved keeps empty ids and is retried on every later write and once
on a short timer after `turn.complete`. A row still unresolved when the file is written carries `step_key`
`turn-<turnId>-<index>`, which the reader uses as the key so the usage still counts; the residual risk is a double
count of that one request if the pull also reads the session.

## 5. Terms

No price. Ships with BurnMon under the repo's licence, free, local. Ending: if Anthropic documents that a person's
mods will not run in cloud sessions, the mod is retired again and this plan closes.

## 6. Risks

| Risk | Source | Answered by |
|---|---|---|
| Code that does nothing for months | Note, decision 1 | Section 0 and 4 say so; the reason line shows "no delivery yet" |
| Double counting with the pull | New | H2, lane step 1, the de-duplication test |
| The mod acts in local Claude Code | Measured: personal mods load locally | Entrypoint guard, H3 |
| `device_commit_files` prompts or is refused from a mod | Note, VERIFY | H1 kill signal; the row file stays in the session's outputs |
| The drop folder is not connected to the session | Old plugin README | README says to add `~\.burnmon` to the Project's folders |
| Employer names in BurnMon files | The ClientA wall (AGENTS.md) | No org or connector names in rows, logs or README; debug logs stay out of the tree |

## 7. Assumptions register

| Assumption | Source | Test | Status |
|---|---|---|---|
| A person's mods will load in Cowork (Cloud) one day | none | The first delivered file | ASSUMED |
| `turn.step` carries usage per request with the model | Mods docs; laptop probe 2026-11-23 | Probe file | CONFIRMED |
| `turn.step` carries a request or message id | Not seen | Lane step 1 | KILLED 2026-11-24: no id; the key comes from the transcript row (section 4a) |
| The step's transcript row can be found by `uuid` through `transcriptPath` | Laptop probe 2026-11-24 | Section 4a | CONFIRMED on the laptop; ASSUMED in the cloud |
| `CLAUDE_CODE_ENTRYPOINT` is `remote_cowork` in every Cowork (Cloud) session | One session measured | Read in each new cloud session the mod runs in | ASSUMED |
| `$.fs.write` creates `/mnt/user-data/outputs` files in the cloud | Laptop run created the path | First cloud run | ASSUMED |
| `device_commit_files` works from a mod without a prompt | Old plugin used a hook, not a mod | First cloud run | ASSUMED |

## 8. Why not the nearest alternatives

The external scan (`04_assets\2026-11-23_external_scan_cloud_usage_routes.md`) found no local route elsewhere:
tokens submits from the cloud VM to its own server, tokscale documents the gap, OpenTelemetry and the analytics
exports need an org admin and leave the machine. The teleport pull is service-blocked (#111387, three CLI versions
tested). Lead 4 found no local usage file. The mod is the only local, documented candidate left.

## 9. What this plan changes elsewhere

`cmd\burnmon\README.md`, `_archive\burnmon-cloud` (one pointer line), `02_roadmap\roadmap.md` (one row, after Wilco's
approval), a hub brief at lane end. The honesty lane lands first; this lane rebases on it.

## Decision block

Decision: build `burnmon-cloud` 2.0.0, a cloud-only usage mod, now, ahead of Anthropic opening mods in cloud
sessions (Wilco, 2026-11-23, against Claude's recommendation to park it). Context: the teleport pull is blocked on
the service side; a person's mods do not load in cloud sessions today. Alternatives: park with a retest, kill.
Consequences: a dormant mod, a reason line that says so, three hypotheses graded on the first cloud delivery.
