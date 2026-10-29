#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
OUT="${1:?Usage: macos/build.sh /absolute/path/to/CCAM.app}"
case "$OUT" in /*.app) ;; *) echo 'Output must be an absolute .app path' >&2; exit 1;; esac
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"
ICON_WORK=$(mktemp -d)
trap 'rm -rf "$ICON_WORK"' EXIT
swift macos/Icon.swift "$ICON_WORK/AppIcon.iconset"
iconutil -c icns "$ICON_WORK/AppIcon.iconset" -o "$OUT/Contents/Resources/AppIcon.icns"
cp LICENSE "$OUT/Contents/Resources/LICENSE"
go build -o "$OUT/Contents/Resources/ccam" ./cmd/ccam
swiftc -parse-as-library -O -target "$(uname -m)-apple-macosx13.0" \
  macos/AccountManager.swift macos/Views.swift macos/App.swift -o "$OUT/Contents/MacOS/CCAM" -framework SwiftUI -framework AppKit -framework UserNotifications
cat > "$OUT/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>CCAM</string>
<key>CFBundleDisplayName</key><string>CCAM 계정 관리</string>
<key>CFBundleIdentifier</key><string>dev.ccam.desktop.beta</string>
<key>CFBundleIconFile</key><string>AppIcon</string>
<key>CFBundleExecutable</key><string>CCAM</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.2.0</string>
<key>CFBundleVersion</key><string>2</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
# CCAM_SIGN_IDENTITY="Developer ID Application: …" signs for distribution
# (hardened runtime, timestamp); without it the app is signed ad hoc for
# local testing. CCAM_NOTARY_PROFILE (a `xcrun notarytool store-credentials`
# profile) then also notarizes and staples the app.
if [ -n "${CCAM_SIGN_IDENTITY:-}" ]; then
  codesign --force --options runtime --timestamp --sign "$CCAM_SIGN_IDENTITY" "$OUT/Contents/Resources/ccam"
  codesign --force --options runtime --timestamp --sign "$CCAM_SIGN_IDENTITY" "$OUT"
  if [ -n "${CCAM_NOTARY_PROFILE:-}" ]; then
    ZIP_WORK=$(mktemp -d)
    ditto -c -k --keepParent "$OUT" "$ZIP_WORK/CCAM.zip"
    xcrun notarytool submit "$ZIP_WORK/CCAM.zip" --keychain-profile "$CCAM_NOTARY_PROFILE" --wait
    xcrun stapler staple "$OUT"
    rm -rf "$ZIP_WORK"
  fi
else
  codesign --force --sign - "$OUT/Contents/Resources/ccam"
  codesign --force --sign - "$OUT"
fi
echo "$OUT"
