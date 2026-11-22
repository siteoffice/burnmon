# burnmon, roadmap

Priority order lives here and only here.

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
21. PROPOSAL 2026-11-21, no date, after the release lane: an optional focus-and-break rhythm in Glim
    or BurnMon Dev (for example, Glim dims after a set focus block while agents keep running).
    Evidence: HBR and BCG 2026 survey of 1,488 full-time US workers, about 14 percent of AI users
    reported brain fry, heavy users overseeing several agents most at risk; Bainbridge 1983,
    "Ironies of Automation". Input for the 2026-12-19 scoring, not a feature. Source:
    `C:\ZND\10_holding\04_assets\2026-11-21_external_scan_herdbell_security_audit_reader_test.md`
    sections 2a item 6, 3 and 6, rec 3.

Full plan with hypotheses, risks and assumptions: `2026-09-22_burnmon_plan.md`.
Architecture (decided): `..\04_assets\2026-09-22_token_monitor_architecture.md`.
Now page and live features (decided): `..\04_assets\2026-09-22_burnmon_now_page_features.md`.
