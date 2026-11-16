# Parked after v0.4.0-alpha.2 (2026-09-26)

Wilco accepted v0.4.0-alpha.2 (`c236162`, branch `burnmon-dev`) as is. These items came out of
the hub review of the phase 5, WS3 and WS2 performance patch reports. Items 1 to 5 are planned
as v0.4.0-alpha.3, later, no date (`roadmap.md` item 9). Never use em dashes anywhere.

## BurnMon Dev (branch `burnmon-dev`)

1. **Minimized RAM.** The Go process peaked at 297.2 MB minimized, against 195.4 MB active and a
   250 MB target. When hidden, only the cheap sampler slows to 10 s; the 3 s process walk keeps
   running and allocates a fresh 2 MB buffer each walk. Slow the walk when hidden, reuse the
   buffer, then measure 5 minutes minimized again.
2. **WebView2 memory target.** Not built in the patch ("ICoreWebView2Controller4 not vendored").
   The interface is in the public `WebView2.h` of Microsoft's `Microsoft.Web.WebView2` NuGet
   package; take the vtable from there. WebView2 peaks at 671 MB active, the largest cost left.
3. **To Do due dates in local time.** Skipped as "unverifiable without a live sign-in". Wilco
   signed in on 2026-09-26, so it can be verified live now.
4. **d9 at 1920x1080.** `.heatmonth` in `cmd\burnmon-dev\page.html` is 9 px wide with
   `overflow:visible`, so month labels overflow their box and trip `check_d9.go`. No real
   scrollbar. Phase 5 never tested 1920 at full size (its screen capped it).
5. **w1 re-run.** Last run failed because another window took the foreground. Re-run solo on an
   idle desktop.
6. Seven minor review findings in the WS2 performance patch report and its hub brief
   `C:\ZND\50_projects\burnmon\04_assets\hub_agent_update_2026-09-26_ws2_performance_patch.md`.

## BurnMon main (shared code)

7. **Watcher buffer overflow.** `internal\watch\watch_windows.go` logs a kernel buffer overflow
   and waits for the 15-minute full rescan. Trigger an immediate rescan of that root instead.
8. **History whole-table load.** WS3 hypothesis 3: `AllEvents()` costs about 3 s on 55k rows,
   periodic only. Listed in "Known gaps" in `03_logs\status_history.md`.
9. **Plan-limits panel.** Parked earlier, no brief yet (idea source
   github.com/Javis603/token-monitor, MIT).
