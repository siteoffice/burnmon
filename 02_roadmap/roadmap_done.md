# burnmon, roadmap, done rows

Rows moved out of `C:\ZND\50_projects\burnmon\02_roadmap\roadmap.md` on 2026-11-16, unchanged, with their original numbers. Open rows stay in the roadmap.

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
11. B1 to B7.3 (2026-10-25 to 2026-10-30), Foreman sprints, specs in
    `C:\ZND\10_holding\05_docs\superpowers\specs\`. **DONE:** all merged into `main` and pushed
    (shared snapshot and keep_history, section groups, Waits on you and Outcomes, first controls,
    docs pass, map and waits, layout fill, list floors, uicheck hygiene, snapshot status line).
12. v0.4.0-beta.1 (2026-10-30). **DONE:** tag `v0.4.0-beta.1` on `775e0b5`, pushed; all three exes
    report `0.4.0-beta.1`; `TestOneVersionClock` keeps them in step. Leftover: prune the merged
    sprint worktrees and branches (Wilco's git step, with the B8 close-out).
