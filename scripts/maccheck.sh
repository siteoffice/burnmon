#!/usr/bin/env bash
set -euo pipefail
# B9 scripted Mac run. Full mode (rental day, real Mac with a screen):
#   bash scripts/maccheck.sh
# CI mode (macos-latest, no display): build, vet and test only.
#   bash scripts/maccheck.sh --ci
# Writes maccheck-out/report.md plus one log per step. Installs nothing.

MODE=full
if [ "${1:-}" = "--ci" ]; then MODE=ci; fi

cd "$(dirname "${BASH_SOURCE[0]}")/.."

OUT=maccheck-out
REPORT="$OUT/report.md"
DOM_CASES="charts/d11 charts/d13 charts/d14 charts/d21 waits/d45"
DEV_LOG="$HOME/Library/Application Support/burnmon/burnmon-dev-app.log"
DONE_LINE="initial collect and backfill done"

FAILED=0
APP_PID=""
DEV_PID=""
DEV_SETTINGS=""
DONE_BEFORE=0

mkdir -p "$OUT"
printf '*\n' > "$OUT/.gitignore"

cleanup() {
  if [ -n "$APP_PID" ]; then kill "$APP_PID" 2>/dev/null || true; fi
  if [ -n "$DEV_PID" ]; then kill "$DEV_PID" 2>/dev/null || true; fi
  return 0
}
trap cleanup EXIT

row() {
  printf '| %s | %s |\n' "$1" "$2" >> "$REPORT"
}

write_header() {
  local gov swv sha
  gov="$(go version 2>/dev/null || echo 'go not found')"
  swv="$(sw_vers -productVersion 2>/dev/null || echo 'n/a')"
  sha="$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
  {
    printf '%s\n' '# maccheck report'
    printf '\n'
    printf '%s\n' "- Mode: $MODE"
    printf '%s\n' "- Arch: $(uname -m)"
    printf '%s\n' "- Go: $gov"
    printf '%s\n' "- macOS: $swv"
    printf '%s\n' "- Commit: $sha"
    printf '\n'
  } > "$REPORT"
}

write_header

# Preflight: report what is missing, install nothing.
MISSING=""
if ! command -v go >/dev/null 2>&1; then MISSING="$MISSING go"; fi
if ! xcode-select -p >/dev/null 2>&1; then MISSING="$MISSING xcode-select"; fi
if [ -n "$MISSING" ]; then
  printf '%s\n' "Preflight failed, missing:$MISSING" | tee -a "$REPORT"
  if ! xcode-select -p >/dev/null 2>&1; then
    printf 'Install the Xcode command line tools: xcode-select --install\n' | tee -a "$REPORT"
  fi
  if ! command -v go >/dev/null 2>&1; then
    printf 'Install Go from https://go.dev/dl/ and run this script again.\n' | tee -a "$REPORT"
  fi
  exit 1
fi

printf '%s\n' '| Step | Result |' '|---|---|' >> "$REPORT"

# step NAME CMD...: run CMD, log to $OUT/<NAME>.log, record ok or FAILED.
step() {
  local name="$1"
  shift
  local log="$OUT/${name//\//-}.log"
  if "$@" > "$log" 2>&1; then
    row "$name" "ok"
  else
    row "$name" "FAILED (see $log)"
    FAILED=1
  fi
}

# Same as step, but it needs a screen, so CI mode records a skip.
window_step() {
  if [ "$MODE" = ci ]; then row "$1" "skipped (ci)"; else step "$@"; fi
}

wait_port() {
  local port="$1" limit="$2" n=0
  while ! nc -z 127.0.0.1 "$port" >/dev/null 2>&1; do
    n=$((n + 1))
    if [ "$n" -ge "$limit" ]; then return 1; fi
    sleep 1
  done
  return 0
}

count_done() {
  local c
  c="$(grep -c "$DONE_LINE" "$DEV_LOG" 2>/dev/null || true)"
  printf '%s' "${c:-0}"
}

build_uicheck() {
  ( cd tools/uicheck && go build -o ../../$OUT/uicheck . )
}

start_burnmon() {
  BURNMON_UICHECK=1 "$OUT/burnmon" > "$OUT/burnmon-app.log" 2>&1 &
  APP_PID=$!
  wait_port 9333 60
}

probe_burnmon() {
  local reply
  reply="$(printf '{"script":"document.title"}\n' | nc -w 10 127.0.0.1 9333 || true)"
  printf '%s' "$reply" | grep -q '"ok":true'
}

stop_burnmon() {
  if [ -n "$APP_PID" ]; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
    APP_PID=""
  fi
}

start_burnmon_dev() {
  DEV_SETTINGS="$(mktemp -d)"
  DONE_BEFORE="$(count_done)"
  BURNMON_DEV_UICHECK=1 BURNMON_DEV_SETTINGS_DIR="$DEV_SETTINGS" "$OUT/burnmon-dev" > "$OUT/burnmon-dev-app.log" 2>&1 &
  DEV_PID=$!
  wait_port 9334 60 || return 1
  local n=0
  while [ "$(count_done)" -le "$DONE_BEFORE" ]; do
    n=$((n + 1))
    if [ "$n" -ge 180 ]; then return 1; fi
    sleep 1
  done
  return 0
}

run_uicheck() {
  BURNMON_DEV_SETTINGS_DIR="$DEV_SETTINGS" "$OUT/uicheck" "$1"
}

stop_burnmon_dev() {
  if [ -n "$DEV_PID" ]; then
    kill "$DEV_PID" 2>/dev/null || true
    wait "$DEV_PID" 2>/dev/null || true
    DEV_PID=""
  fi
}

step build-burnmon env CGO_ENABLED=1 go build -o "$OUT/burnmon" ./cmd/burnmon
step build-burnmon-dev env CGO_ENABLED=1 go build -o "$OUT/burnmon-dev" ./cmd/burnmon-dev
step build-burnmon-cli env CGO_ENABLED=1 go build -o "$OUT/burnmon-cli" ./cmd/burnmon-cli
step build-controlcentre env CGO_ENABLED=1 go build -o "$OUT/controlcentre" ./cmd/controlcentre
step build-uicheck build_uicheck
step vet go vet ./...
step test go test ./... -count=1

window_step start-burnmon start_burnmon
window_step probe-burnmon probe_burnmon
window_step screenshot-burnmon screencapture -x "$OUT/burnmon.png"
window_step stop-burnmon stop_burnmon
window_step start-burnmon-dev start_burnmon_dev
for c in $DOM_CASES; do window_step "uicheck-$c" run_uicheck "$c"; done
window_step screenshot-burnmon-dev screencapture -x "$OUT/burnmon-dev.png"
window_step stop-burnmon-dev stop_burnmon_dev

{
  printf '\n'
  printf '%s\n' '## Manual checks'
  printf '\n'
  printf '%s\n' '- [ ] Window feel: resize the window, check text is sharp on Retina, switch to dark mode'
  printf '%s\n' '- [ ] Menus: Cmd+Q quits, copy and paste work in the gear dialog'
  printf '%s\n' '- [ ] Sleep and wake: close the lid for 2 minutes, sampling resumes, no rate spike'
  printf '%s\n' '- [ ] Second screen: move and resize the window across both screens'
  printf '%s\n' '- [ ] Screen Recording permission is granted for screencapture (the screenshots are not black)'
  printf '%s\n' '- [ ] OF25 banner: the three B9 item 5 checks'
  printf '%s\n' '- [ ] Control Centre window: run maccheck-out/controlcentre --root <a folder with a siteoffice.json>, walk every page in light and dark at 60, 80 and 100 percent'
} >> "$REPORT"

cat "$REPORT"
exit "$FAILED"
