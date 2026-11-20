#!/usr/bin/env bash
set -euo pipefail
# Builds the startable macOS zip of one release, on a Mac (Xcode command line
# tools: cgo, sips, iconutil, ditto):
#   bash scripts/package.sh <tag> [arm64|amd64]
# The three window programs are built with cgo (the native window, not the
# browser mode of release.yml's bare files) and each goes into an app bundle:
# BurnMon.app, BurnMon Dev.app and Control Centre.app. The zip
# dist/burnmon-<tag>-macos-<arch>.zip holds them with burnmon-cli,
# burnmon.example.json and README-FIRST.txt. The bundles are unsigned: signing
# and notarizing wait on the Apple enrolment (Ladder row OF26).
# The script ends by listing the zip and failing when a part is missing.

if [ $# -lt 1 ] || [ -z "$1" ]; then
  echo "usage: bash scripts/package.sh <tag> [arm64|amd64]" >&2
  exit 2
fi
TAG="$1"
ARCH="${2:-}"
if [ -z "$ARCH" ]; then
  case "$(uname -m)" in
    arm64) ARCH=arm64 ;;
    x86_64) ARCH=amd64 ;;
    *) echo "unknown machine $(uname -m); pass arm64 or amd64" >&2; exit 2 ;;
  esac
fi
if [ "$ARCH" != arm64 ] && [ "$ARCH" != amd64 ]; then
  echo "arch must be arm64 or amd64, not $ARCH" >&2
  exit 2
fi
# The bundle version: the tag without its leading v (v0.4.0-beta.9 -> 0.4.0-beta.9);
# CFBundleVersion takes only the numbers before the first dash (0.4.0).
VERSION="${TAG#v}"
NUMERIC="${VERSION%%-*}"

cd "$(dirname "${BASH_SOURCE[0]}")/.."

NAME="burnmon-$TAG-macos-$ARCH"
WORK="dist/pkg-$ARCH"
STAGE="$WORK/$NAME"
ZIP="dist/$NAME.zip"
mkdir -p "$STAGE"
if [ -e "$ZIP" ] || [ -n "$(ls -A "$STAGE")" ]; then
  echo "$ZIP or $STAGE already holds files; use a clean checkout" >&2
  exit 2
fi

build() { # build <out> <package>
  CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" go build -trimpath -ldflags "-s -w" -o "$1" "$2"
}

# icns_from_pngs <out.icns>: BurnMon and BurnMon Dev share winres/icon-*.png
# (the Windows icon). iconutil wants an .iconset folder with fixed names; the
# 512 px slot is scaled up from 256 px by sips.
icns_from_pngs() {
  local set="$WORK/burnmon.iconset"
  mkdir -p "$set"
  cp winres/icon-16.png "$set/icon_16x16.png"
  cp winres/icon-32.png "$set/icon_16x16@2x.png"
  cp winres/icon-32.png "$set/icon_32x32.png"
  cp winres/icon-64.png "$set/icon_32x32@2x.png"
  cp winres/icon-128.png "$set/icon_128x128.png"
  cp winres/icon-256.png "$set/icon_128x128@2x.png"
  cp winres/icon-256.png "$set/icon_256x256.png"
  sips -z 512 512 winres/icon-256.png --out "$set/icon_256x256@2x.png" >/dev/null
  sips -z 512 512 winres/icon-256.png --out "$set/icon_512x512.png" >/dev/null
  iconutil -c icns "$set" -o "$1"
}

# bundle <App Name> <executable> <package> <bundle id> <icns file>
bundle() {
  local app="$STAGE/$1.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  build "$app/Contents/MacOS/$2" "$3"
  cp "$5" "$app/Contents/Resources/$2.icns"
  cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$1</string>
  <key>CFBundleDisplayName</key><string>$1</string>
  <key>CFBundleIdentifier</key><string>$4</string>
  <key>CFBundleExecutable</key><string>$2</string>
  <key>CFBundleIconFile</key><string>$2.icns</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$NUMERIC</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
  plutil -lint "$app/Contents/Info.plist" >/dev/null
}

icns_from_pngs "$WORK/burnmon.icns"
bundle "BurnMon" burnmon ./cmd/burnmon dev.zerononsense.burnmon "$WORK/burnmon.icns"
bundle "BurnMon Dev" burnmon-dev ./cmd/burnmon-dev dev.zerononsense.burnmon-dev "$WORK/burnmon.icns"
bundle "Control Centre" controlcentre ./cmd/controlcentre dev.zerononsense.controlcentre winres/controlcentre.icns
build "$STAGE/burnmon-cli" ./cmd/burnmon-cli
cp burnmon.example.json "$STAGE/"
cp packaging/macos/README-FIRST.txt "$STAGE/"

# ditto keeps the bundle layout and the executable bits; --keepParent puts
# everything under one folder named like the zip.
ditto -c -k --keepParent "$STAGE" "$ZIP"

# Check the zip itself, not the stage folder.
LIST="$(unzip -Z1 "$ZIP")"
MISSING=0
for want in \
  "BurnMon.app/Contents/MacOS/burnmon" "BurnMon.app/Contents/Info.plist" "BurnMon.app/Contents/Resources/burnmon.icns" \
  "BurnMon Dev.app/Contents/MacOS/burnmon-dev" "BurnMon Dev.app/Contents/Info.plist" "BurnMon Dev.app/Contents/Resources/burnmon-dev.icns" \
  "Control Centre.app/Contents/MacOS/controlcentre" "Control Centre.app/Contents/Info.plist" "Control Centre.app/Contents/Resources/controlcentre.icns" \
  "burnmon-cli" "burnmon.example.json" "README-FIRST.txt"; do
  if ! printf '%s\n' "$LIST" | grep -Fqx "$NAME/$want"; then
    echo "missing in $ZIP: $NAME/$want" >&2
    MISSING=1
  fi
done
for exe in "BurnMon.app/Contents/MacOS/burnmon" "BurnMon Dev.app/Contents/MacOS/burnmon-dev" "Control Centre.app/Contents/MacOS/controlcentre" "burnmon-cli"; do
  if ! lipo -archs "$STAGE/$exe" | grep -qw "$( [ "$ARCH" = amd64 ] && echo x86_64 || echo arm64 )"; then
    echo "$exe is not built for $ARCH: $(lipo -archs "$STAGE/$exe")" >&2
    MISSING=1
  fi
done
if [ "$MISSING" -ne 0 ]; then exit 1; fi
printf '%s\n' "$LIST"
echo "packaged $ZIP ($(wc -c < "$ZIP" | tr -d ' ') bytes)"
