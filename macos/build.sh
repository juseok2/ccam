#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
OUT="${1:?Usage: macos/build.sh /absolute/path/to/CCAM.app}"
case "$OUT" in /*.app) ;; *) echo 'Output must be an absolute .app path' >&2; exit 1;; esac
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"
cp LICENSE "$OUT/Contents/Resources/LICENSE"
go build -o "$OUT/Contents/Resources/ccam" ./cmd/ccam
swiftc -parse-as-library -O -target "$(uname -m)-apple-macosx13.0" \
  macos/AccountManager.swift -o "$OUT/Contents/MacOS/CCAM" -framework SwiftUI -framework AppKit
cat > "$OUT/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>CCAM</string>
<key>CFBundleDisplayName</key><string>CCAM 계정 관리</string>
<key>CFBundleIdentifier</key><string>dev.ccam.desktop.beta</string>
<key>CFBundleExecutable</key><string>CCAM</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
codesign --force --sign - "$OUT/Contents/Resources/ccam"
codesign --force --sign - "$OUT"
echo "$OUT"
