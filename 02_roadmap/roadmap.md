# burnmon, roadmap

Priority order lives here and only here.

1. v0.1 (2026-10-17): schema and SQLite store, Codex adapter, the live Now page.
   Spec: `2026-09-22_v0.1_spec.md`. Session prompt: `2026-09-22_v0.1_session_prompt.md`.
   Steps 1 to 3 landed 2026-09-22 (`693ea5a`). Fixes from the first live run:
   `2026-09-22_v0.1.1_fix_spec.md` (memory, Codex missing, running chart, labels, windows, copy).
2. v0.1.2 patch (week 39): Codex sessions appear without Refresh, smooth running chart,
   version strings and STATUS. Spec: `2026-09-22_v0.1.2_patch_spec.md`.
3. v0.2 (2026-11-14), one release, two sessions a week: migrations and tool_calls, five
   tabs (Now default, History with filters, Sessions, Tools, About), vendor strip, English
   only, owner split, insight package (re-prefill, compaction, runway, spike drawer),
   forecast behind a visible gate, Hermes and Copilot CLI adapters, Copilot VS Code OTel
   check. Spec: `2026-09-22_v0.2_spec.md`. Session prompts, one per session, with the
   checklist: `2026-09-22_v0.2_session_prompts.md`. Grilled 2026-09-22. **DONE 2026-09-23,
   tagged `v0.2.0`, all sixteen spec items shipped, release-candidate Done-when and VERIFY
   pass clean (46A), no fails carried into the tag.**
4. v0.2.1 patch: Refresh went "Not Responding" (98.5% CPU, 1,438 MB, 175 MB/s disk).
   Measured root cause: the Sessions tab fired one bmSessionInsight call per kept session
   unconditionally on every page load (real store: ~1,700 sessions), all serialised through
   the store's one connection and blocking the WebView2 UI thread call by call; Collect
   itself was fast (1.2-2.6s) and not the driver. Spec:
   `2026-09-23_v0.2.1_hang_patch.md`. **DONE 2026-09-23, tagged `v0.2.1`.** Not Responding
   and the CPU pin are fixed (bmLive stayed under 50ms throughout every measured run);
   peak WorkingSet dropped from ~1.4-1.7 GB to ~900 MB, short of the 400 MB Done-when
   target and left open in `STATUS.md`'s Known gaps for v0.3 or a follow-up patch.
5. v0.2.2 patch: Now page fixes from Wilco's live review (three screenshots, 2026-09-23
   around 12:00). Turn drawer could not be closed (root cause: bmTurn ran synchronously on
   the WebView2 UI thread, same mechanism as the v0.2.1 hang, now async plus an Esc handler);
   Live burn chart rebuilt as clustered per-minute bars (BucketSeconds 10 to 60, one hue
   family per vendor); more gap above the session cards; vendor strip links de-styled to
   plain numbers with a hover tooltip; the context-window book corrected (Opus 5.5, Sonnet 5
   and Fable 5.1 run a native 1M-token window, not 200K, re-checked live against
   platform.claude.com/docs; the Opus key itself was also wrong, "claude-opus-5" instead of
   the real "claude-opus-5-5"); the Codex "(model unknown)" card traced to scanHeaderMeta
   never re-reading turn_context on an incremental parse, now carries the header-scanned
   model forward the same way it already did for surface and cwd. Spec:
   `2026-09-23_v0.2.2_now_page_patch.md`. **DONE 2026-09-23, committed locally, tag `v0.2.2`
   not yet pushed, see SESSION_LOG.md for the commands.**
6. v0.2.3 patch: v0.2.1 and v0.2.2 were both verified by proxy (no session could see or
   click the real WebView2 window); W0 built a real harness instead (`tools\uicheck`, Win32
   screenshot/click/key plus a dev-only eval channel `cmd\burnmon\uicheck_devserver.go`
   opens under `BURNMON_UICHECK=1`), then W1 to W8 reproduced and fixed against the real
   window: startup blank/Not Responding (unbounded filesystem scan ran before `w.Run()`,
   moved into the startup goroutine); the turn drawer's Close button and overlay click
   (inline `onclick="closeTurnDrawer()"` resolving in the wrong scope, the whole page script
   is one IIFE; Esc already worked via `addEventListener`); a horizontal scrollbar on long
   unbroken paths in the drawer (table-layout:fixed plus overflow-wrap:anywhere); duplicate
   "recent session" legend entries for a session that went idle within the chart's 30-minute
   window but outside `live.BuildSnapshot`'s shorter running window (id-prefix fallback
   label instead); legend order (vendor, surface, model, project, alphabetical, cost last);
   repeating Now-chart axis ticks (a two-decimal `tokPrecise`, scoped to that chart only);
   skipped X-axis minute labels (every minute now, rotate only under 36px/minute); the cost
   axis not hiding with its series (`display:'auto'`). Spec:
   `2026-09-23_v0.2.3_window_check_patch.md`. **DONE 2026-09-23, committed locally, tag
   `v0.2.3` not yet pushed, see SESSION_LOG.md for the commands.**
7. v0.3 (2026-10-09, moved from 2026-12-12), one release, two sessions a week plus a
   release-candidate pass: price books, per-vendor cost, dev and business switch, client
   map (owner then client), active time, export and merge, Copilot VS Code (OTel file),
   macOS and Linux in browser mode, Now page fixes and monitor mode. Spec:
   `2026-09-23_v0.3_spec.md`. Session prompts with the checklist:
   `2026-09-23_v0.3_session_prompts.md`. Grilled 2026-09-23. **DONE 2026-09-24, tagged
   `v0.3.0`, 15 days early, all Done-when items pass against the built exe and the real
   local store, no fail carried into the tag** (V3-6, `SESSION_LOG.md`).
8. WS1 to WS3 plus BurnMon Dev (2026-09-24 to 2026-09-26). **DONE:** v0.3.1 cleanup
   (`1291ef9`), v0.3.2 shared ingest performance and local time everywhere (`69a071e`),
   BurnMon Dev `burnmon-dev.exe` v0.4.0-alpha.2 on branch `burnmon-dev` (`c236162`), accepted
   by Wilco 2026-09-26 and merging into `main`. Briefs: `2026-09-24_ws1_burnmon_cleanup.md`,
   `2026-09-24_ws2_burnmon_dev.md`, `2026-09-26_ws3_shared_ingest_performance.md`,
   `2026-09-26_ws2_performance_patch.md`.
9. v0.4.0-alpha.3, later, no date: BurnMon Dev minimized RAM (297 MB against a 250 MB
   target), WebView2 memory target, To Do due dates in local time, d9 at 1920x1080, w1 solo
   re-run. Items and main-side follow-ups: `2026-09-26_parked_after_alpha2.md`.
10. After the park (2026-11-01), no date, two findings logged 2026-10-07:
    a. Browser mode (darwin, linux): the Refresh button stays on "Rebuilding..." because
       `window.ccRefresh` is not bound outside the WebView2 window (`cmd\burnmon\main_other.go`).
       Hide the button in browser mode and say the page reloads with the next rewrite.
       **Replaced 2026-10-21 (Wilco):** PR #4 (merkendiep) adds a localhost-only bridge on
       macOS and Linux that serves the live functions, Refresh included. Accepted on three
       conditions (per-run secret, README says it plainly, Windows verify after the park).
       Decision: `2026-10-21_pr4_macos_bridge_decision.md`.
    b. Auto theme: a third theme state next to Dark and Light that follows the system
       (`prefers-color-scheme`), in `internal\report\template.html` and
       `cmd\burnmon-dev\page.html`.
    c. Design session tag (decided 2026-10-11): tag Claude Code sessions that publish a Design
       canvas (`project/canvas.json`, `*.dc.html` in Artifact tool input) as `design`, session
       level, agent stays `claude-code`. Spec: `2026-10-11_design-session-tag.md`.
    d. Cloud Cowork sessions (decided 2026-10-12, roadmap row over README note): cloud-run Cowork
       sessions leave no local transcript, so BurnMon undercounts Cowork. Plan: a BurnMon companion
       plugin whose Stop hook writes a usage-only, deduplicated JSONL inside the cloud container and
       delivers it to a connected laptop folder that BurnMon reads; until then show "N cloud Cowork
       sessions, not measured" from `remote-session-spaces.json`. Gated on live tests (hook fires in
       cloud, hook-driven delivery: both passed 2026-10-12). Spec: `2026-10-12_cowork_cloud_bucket_spec.md`; test plugin:
       `..\tools\burnmon-cloud-probe` (removed 2026-10-12, in git history at `be652f9`). Second park exception (Wilco, 2026-10-12, "build now"): built the same
       day, plugin `..\tools\burnmon-cloud`, agent `cowork-cloud`. **DONE 2026-10-12** (`a249bc5`), live in
       BurnMon Dev the same evening. Not built yet: the "not measured" counter.
       Research: `..\04_assets\2026-10-12_cloud_cowork_capture_research.md`.
       Proof: `..\04_assets\2026-10-12_cloud_cowork_sessions_proof.md`.
    e. Bug, found 2026-10-12: the live watcher counts a local Cowork session's `audit.jsonl` as a
       second session (shown as `#udit`). `FindJSONL` skips `audit.jsonl`, `outputs` and `uploads`; the
       watcher (`internal\watch\watch_windows.go`, `tail.go` `Track`) only checks the `.jsonl` suffix. Fix:
       one shared path filter for both, a test first, then purge stored rows of session key `audit`.
       Park exception (Wilco, 2026-10-12). Code done 2026-10-12, uncommitted: `scan.IsTranscriptPath`,
       `Store.PurgeAuditSessions`, run on every Collect.
       Proof: same file as row d.
11. B1 to B7.3 (2026-10-25 to 2026-10-30), Foreman sprints, specs in
    `C:\ZND\10_holding\05_docs\superpowers\specs\`. **DONE:** all merged into `main` and pushed
    (shared snapshot and keep_history, section groups, Waits on you and Outcomes, first controls,
    docs pass, map and waits, layout fill, list floors, uicheck hygiene, snapshot status line).
12. v0.4.0-beta.1 (2026-10-30). **DONE:** tag `v0.4.0-beta.1` on `775e0b5`, pushed; all three exes
    report `0.4.0-beta.1`; `TestOneVersionClock` keeps them in step. Leftover: prune the merged
    sprint worktrees and branches (Wilco's git step, with the B8 close-out).
13. B8, the agent half: agent view behind the npm shim, a new gate on the page in under 2 s, Waits
    on you titles and a collapsed Done group, every control once for real (Attended), a toast for
    Blocked and Your turn. Spec: `2026-10-30_b8_agent_half_spec.md` (Foreman copy
    `2026-10-30-burnmon-b8-agent-half.md`, `spec_lint.py` exit 0). Next to run.
14. B9, native macOS window (Wilco 2026-10-30: "I need the native macOS one", right after B8): BurnMon
    and BurnMon Dev in their own window on macOS instead of the browser plus the 127.0.0.1 server of
    PR #4. WebView2 is Windows-only (`cmd\burnmon\main_other.go`) and every build is `CGO_ENABLED=0`,
    so it needs a cgo webview library built on a Mac or a macOS CI runner. Library: webview_go
    (Wilco 2026-10-30; research `..\04_assets\2026-10-30_b9_macos_webview_research.md`). Test plan (decided by Wilco, recorded here 2026-10-30):
    rent a Mac online for 24 hours and test everything there; renting is Wilco's step (it spends
    money). Everything that can be ready before the rental is ready before it: code, tests, a macOS
    compile check in CI, and one scripted test run, so the 24 hours go to testing. Both burnmon and
    BurnMon Dev get the window (Wilco 2026-10-30). Spec: `2026-10-30_b9_native_macos_window_spec.md`.
15. B10, cost per outcome (decided by Wilco 2026-10-30, ahead of plan limits, after the macOS
    window): tokens and cost per merged sprint and per closed gate, from the sprint links and
    outcomes BurnMon already has; plus the burn-spike toast, built on B8's toast. Evidence for H2
    and H3. No spec yet.
16. Plan-limits research, before any build: which vendors write seat or allowance state to local
    disk in a documented form. A brief in a plain chat, no code. It decides whether a plan-limits
    panel can exist under the no-undocumented-endpoints rule. No brief yet.
17. Native Linux window, after the macOS one, same approach. Linux is untested on real hardware today.
18. Station polish (layout, seated poses, labels), after the native windows.
19. Performance debt, after B10: History re-reads the whole store on every filter change (about 3 s
    at 55,000 events), the Station CPU climb (cause not found), minimized RAM 297 MB against
    250 MB, uicheck w1 red.
20. Decision 2026-12-19.

Full plan with hypotheses, risks and assumptions: `2026-09-22_burnmon_plan.md`.
Architecture (decided): `..\04_assets\2026-09-22_token_monitor_architecture.md`.
Now page and live features (decided): `..\04_assets\2026-09-22_burnmon_now_page_features.md`.
