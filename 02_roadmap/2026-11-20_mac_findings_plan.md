# Mac findings lane: plan file and turn log

Lane file: `C:\ZND\10_holding\02_roadmap\2026-11-20_mac_findings_lane.md`. Branch: `work/wilco-de-tree/mac-findings-lane`.
Starts from: `main` at `aac8328` (the land of pull request 64, the Mac fix lane). Run mode: Unattended.

## Baseline (T1, on aac8328)

Measured before any edit, from `C:\ZND\50_projects\burnmon` in PowerShell:

- `go vet ./...`: exit 0.
- `go test ./... -count=1`: exit 0, 50 packages ok, no FAIL line.
- `.\build.ps1`: exit 0, the four exes built.

## T1: D12 and D14, the numbers and the Tools text

### D12, the cause

Measured with the new test `C:\ZND\50_projects\burnmon\internal\history\sessions_agree_test.go`, which parses the fixture
`testdata\codex\three-turns.jsonl` (one Codex session, three turns: two on `gpt-5.6-terra`, one on `gpt-6-astra`) and
prices it with the compiled-in defaults. On the start commit: History week 2026-W38 0.115000 USD, Sessions 0.141600 USD.
At the page's default rate 0.92 that is EUR 0.11 and EUR 0.13, the two amounts of the Mac test; BurnMon Dev showed the
Sessions figure in dollars, $0.14. So History is the amount that differed, as the hub inferred, and BurnMon Dev and
Sessions showed one amount in two currencies.

Why they differ (read in code): History prices through `pricing.Config.CostForEvents`, which reads the dated OpenAI price
book `internal\pricing\books\openai_api.json`. That book leaves out `gpt-5.6-terra` and `gpt-5.6-luna` on purpose: on
2026-09-29 they were no longer on OpenAI's pricing page, so they were left out rather than guessed (`SESSION_LOG.md`, the
V3-1 paragraph). `pricing.Config.EventCost`, which Sessions (`internal\dataset\fromstore.go`), live and BurnMon Dev
(`internal\live\live.go`) use per turn, still priced openai turns through the older `OpenAIPrices` map, which kept terra
at $2 / $0.20 / $12. Two tables for one fact, against "one function so the numbers agree everywhere" (C2). A defect.

### D12, the repair

- `internal\pricing\cost.go`: a new `openAIEventCostUSD` prices one turn from the book; `openAIAPICostUSD` (History) and
  `EventCost` (Sessions, live, BurnMon Dev) both use it. A model the book lacks is "no price" everywhere and its tokens
  count as unpriced; its label still comes from the old map, so Sessions keeps the name "GPT-5.6 Terra".
- `internal\devexport\devexport.go`: `turnCostUSD` prices openai turns through `EventCost` too, so the BurnMon Dev
  export uses the same book.
- Effect: a Codex session on `gpt-5.6-terra` now shows only its book-priced turns in Sessions and BurnMon Dev, as it
  already did in History. For the fixture all three now read 0.115 USD (EUR 0.11). Book and old map agree on label and
  rate for `gpt-6-astra` and `gpt-5.6-sol` (read), so no other Codex amount moves.
- Not touched: Claude amounts. BurnMon Dev shows the subscription-share basis for Claude turns and History the headline
  basis (API list unless a subscription is configured); that is by design and not part of D12.

### D12, the currency

BurnMon keeps its currency (`UI.cur`) and rate (`UI.fx`) only in its page storage (`localStorage` in
`internal\report\template.html`); no file both apps read holds the choice. So BurnMon Dev now shows euro at BurnMon's
default rate 0.92: `cmd\burnmon-dev\page.html` has one `fmtCost(usd)` with `EUR_PER_USD = 0.92`. The fixed `$` of
`fmtCost`, `secEuro` (which read `cost.eur` from the snapshot) and the `$` before the header burn rate are gone; every cost
on the page goes through `fmtCost`.

Gaps, named: (1) a user who sets BurnMon to USD, or another rate, still sees euro at 0.92 in BurnMon Dev; closing it needs
the choice in a shared file. (2) The agent map facts are written in Go from the snapshot's `cost.eur`, at `fx_usd_eur`
from `burnmon.json` (default 0.92): same currency, but a changed `fx_usd_eur` moves them and not `fmtCost`. (3) The
Markdown exports of BurnMon Dev (`fmtCost` in `internal\devexport\devexport.go`) still print dollars; they are files, not
the window, and stay as they are in this turn.

### D14

Read in code: only the claude adapter (`internal\adapter\claude\claude.go`) fills `Event.Tools`, so this build counts tools
for Claude Code and Cowork sessions. The Codex adapter keeps its tool calls as `schema.ToolCall` for the turn detail, not
for the Tools page, so the Codex fixture with tool calls shows none there. Its two tools (`exec`,
`request_user_input_async`) are built-in tools, which the Tools page does not list for any agent. Not repaired: counting
Codex tools on the Tools page is new work, not a defect of this text. The text `tools_no_data_html` in
`internal\report\template.html` now reads: "No tool data yet. Tools are counted for Claude Code and Cowork sessions only.
Sessions read before tool counting show nothing until the cache is rebuilt." No golden file holds the text.

### Files changed

`internal\pricing\cost.go`, `internal\devexport\devexport.go`, `cmd\burnmon-dev\page.html`, `internal\report\template.html`;
tests `internal\history\sessions_agree_test.go` (new), `cmd\burnmon-dev\currency_page_test.go` (new),
`internal\report\tools_text_test.go` (new), `internal\dataset\fromstore_test.go` (changed); this plan file.

### Tests

- `TestWeekCostEqualsSessionCost_OneCodexSession`: failed on aac8328 (0.1150 against 0.1416 USD), passes now.
- `TestPageCostOneFunctionOneCurrency`: failed on aac8328 (dollar sign, `secEuro`, no `EUR_PER_USD`), passes now. Extended
  after the review for the header `$`; that part failed first too.
- `TestToolsNoDataTextNamesNoVersion`: failed on aac8328 (names v0.5, no harness), passes now.
- Changed earlier test: `TestSessionsFromEventsPricesOpenAIEvents` in `internal\dataset\fromstore_test.go` priced a turn
  on `gpt-5.6-terra` from the old map. It now uses `gpt-5.6-sol`, a book model, with the same assertions and the sol rates.
  Reason: terra is not in the book, by the 2026-09-29 decision.
- gofmt: `internal\dataset\fromstore_test.go` is not gofmt-clean at lines 19 and 25 already on aac8328; those lines are not
  this turn's and stay as they are.

### uicheck

`.\scripts\uicheck.ps1` and `.\scripts\uicheck-dev.ps1` ran on this laptop's screen and both exit 1. Two cases fail:
`w1` ("window looked blank/unpainted 1.5s after launch") and `station/d28` (the Station is 964 to 1412 px tall in the
four looks, taller than the 844 px of `#pageScroll`). Every other group passed, the Control Centre groups exit 0. A/B:
with the two pages of aac8328 put back and rebuilt, both cases fail the same way with the same heights, so they are not
caused by this turn. Both were rerun once on this turn's build and failed again. Cause not looked into in this turn.

### Choices made unattended

- Align on the book, not on the old map: adding terra back to the book would price it at a rate OpenAI no longer
  publishes, which the 2026-09-29 decision refused. Question for Wilco in the hub brief: add `gpt-5.6-terra` and
  `gpt-5.6-luna` to the book at the old rates, or keep them "no price".
- Review (sonnet, fresh): found the header `$` of the burn rate (repaired with a failing test first) and the dollar
  `fmtCost` of the Go exports (named above, not changed). No test made weaker; the terra-to-sol move is named above.

## T2: D4 and D6, the first window and the gear view

### D4, the first window

Read: the three `window_darwin.go` files called `SetSize` with a fixed 1280 x 860 (BurnMon, BurnMon Dev) or 1536 x 864
(Control Centre). On Windows `go-webview2` (`webview.go`, `CreateWithOptions`) centres on `SM_CXSCREEN` and
`SM_CYSCREEN` and never limits the size (for a screen smaller than the window the centring even underflows, it is an
unsigned subtraction), so the gap D4 is also there on Windows for a small screen; no app limited it. The BurnMon Dev
restore clamp (`cmd\burnmon-dev\windowstate.go`) only covers a saved window.

Repair: `winbind.FitFirstWindow(wantW, wantH, visW, visH)` in `C:\ZND\50_projects\burnmon\internal\winbind\firstwindow.go`
returns the wanted size limited to the visible area minus `FirstWindowMargin` (64 px per axis: title bar and a gap). A
visible size of 0, or one that cannot hold the margin, returns the wanted size. `winbind.FirstWindowSize` reads the visible
area through `VisibleSize`: `visible_windows.go` (`SPI_GETWORKAREA`, work area of the primary monitor),
`visible_darwin.go` (cgo, `[NSScreen mainScreen].visibleFrame`, in points like `SetSize`), `visible_other.go` (0, 0).
The three `window_darwin.go` files and the three `window_windows.go` files use it. The saved window of BurnMon Dev is
unchanged: a saved size still wins.

Tests, first, seen failing (build failed on the missing function): `TestFitFirstWindow` in
`internal\winbind\firstwindow_test.go`, for 1024 x 768, 1470 x 956 and 1920 x 1080 with 1280 x 860 and 1536 x 864, plus
the two fallbacks. Results: 1024 x 768 gives 960 x 704 for both; 1470 x 956 gives 1280 x 860 and 1406 x 864; 1920 x 1080
changes nothing.

Not measured: a window on a Mac or on a small Windows screen (built, not seen). `visible_darwin.go` cannot be compiled
on this laptop; the `macos` check on the pull request is its first compile. The reviewer notes that `NSScreen` is
read from the Go main goroutine, not guaranteed thread 0; a read-only query, untested. DPI: the `.syso` manifests of the
exes carry a DPI awareness entry (found by grep, not decoded), so the work area and the window size should both be in
physical pixels; inferred, not measured at 125 or 150 percent.

### D6, the gear view

Measured on this laptop at the real window (1264 x 821 CSS px, scale 1), through the dev eval channel with a temp settings
folder: the gear pane `#secSettings` has `scrollHeight` 1488, `clientHeight` 737, `max-height` 741 px
(`calc(100vh - 80px)`) and `overflow-y:auto`. At a 1024 x 768 screen the window is at most 704 px outer, so the pane
cap is about 585 to 600 px: it does not fit. The pane already scrolled, but with the default scrollbar, which macOS hides
until a scroll. Repair: `#secSettings` gets `overflow-x:hidden` and the scrollbar rules of `.ticker`
(`scrollbar-width:thin`, 8 px `::-webkit-scrollbar` with the same track and thumb colours), which keeps it visible; no
other pane is touched. Test first, seen failing: `TestGearPaneHasTheHouseScrollbar` in
`cmd\burnmon-dev\gear_scrollbar_page_test.go`. Not measured: the scrollbar in a window at 1024 x 768 (built, not seen).

### Files changed

`internal\winbind\firstwindow.go`, `firstwindow_test.go`, `visible_windows.go`, `visible_darwin.go`, `visible_other.go`;
`cmd\burnmon\window_darwin.go`, `window_windows.go`; `cmd\burnmon-dev\window_darwin.go`, `window_windows.go`, `page.html`,
`gear_scrollbar_page_test.go`; `cmd\controlcentre\window_darwin.go`, `window_windows.go`; this plan file. That is 14 files
against the budget of 12 (L7); the new files are the steps themselves, nothing was left out.

### uicheck

`.\scripts\uicheck-dev.ps1`: `fit` and `charts` ok, `station` red on `station/d28` as on `aac8328` (the baseline, same
message, the Station taller than `#pageScroll` on this screen). The groups after it never run when one is red, so
`layout pct panes map waits shell` were run by name: all pass. `layout/d43` failed once with "the gear did not open"
(a real mouse click on `#secGear`), and passed in two reruns and in the full run. `uicheck.ps1` and
`uicheck-centre.ps1` were not run: no page of BurnMon or the Control Centre changed, only their window code.

### Choices made unattended

- Margin 64 px per axis, one number for all three apps (the hub named none).
- Windows limited too, with the same function (step 3 found the gap).
- The gear scrollbar is the `.ticker` style on the pane itself, not on an inner element.
- Review (sonnet, fresh): no behaviour change outside D4 and D6, no test weakened; it named the Mac thread question and
  the DPI inference above, both left as written.

## Turn log

T1: done; commit 93bd7bc; measured: go vet 0, go test 50 packages ok and 0 FAIL, build.ps1 0, linux and darwin build and vet 0 with CGO off, uicheck.ps1 and uicheck-dev.ps1 exit 1 on w1 and station/d28, the same on aac8328's pages (A/B); open: the terra and luna price question for Wilco, BurnMon Dev cannot read BurnMon's currency choice (euro at 0.92), the Dev Markdown exports still print dollars, w1 and station/d28 fail on this screen before and after this turn (built, not seen in a window)

