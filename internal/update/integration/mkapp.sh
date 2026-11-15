#!/bin/bash
# mkapp.sh <binary> <out.app> <short-version> <bundle-id> <sign: devid|devel|adhoc|none>
# Assembles a minimal .app around a Go binary and signs it (no timestamp server:
# nothing leaves the machine). DEVID_HASH is the SHA-1 of the Developer ID
# Application identity in the login keychain (read from the environment); DEVEL_HASH is an Apple Development
# identity of the same team (optional, used only by the "devel" mode).
set -euo pipefail
bin=$1; app=$2; ver=$3; id=$4; mode=$5
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin" "$app/Contents/MacOS/testapp"
echo "resource for $ver" > "$app/Contents/Resources/data.txt"
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>testapp</string>
<key>CFBundleIdentifier</key><string>$id</string>
<key>CFBundleName</key><string>Codepass</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$ver</string>
<key>CFBundleVersion</key><string>$ver</string>
<key>LSUIElement</key><true/>
</dict></plist>
EOF
case $mode in
  devid) codesign --force --deep --sign "$DEVID_HASH" --timestamp=none --identifier "$id" "$app" ;;
  devel) codesign --force --deep --sign "$DEVEL_HASH" --timestamp=none --identifier "$id" "$app" ;;
  adhoc) codesign --force --deep --sign - --identifier "$id" "$app" ;;
  none)  ;;
esac
