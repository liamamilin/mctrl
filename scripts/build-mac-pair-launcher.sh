#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PROFILE=${PROFILE:-v1}
APP_VERSION=${APP_VERSION:-1.0.0}
case "$PROFILE" in
  v1)
    APP_NAME="Mctrl Pair"
    BUNDLE_ID="com.mctrl.pair-launcher"
    BIN_DIR="$ROOT/bin"
    PROFILE_ARGS="--profile v1"
    ;;
  v[2-9])
    PROFILE_LABEL=$(printf '%s' "$PROFILE" | tr '[:lower:]' '[:upper:]')
    APP_NAME="Mctrl $PROFILE_LABEL Pair"
    BUNDLE_ID="com.mctrl.$PROFILE.pair-launcher"
    BIN_DIR="$ROOT/bin/$PROFILE"
    PROFILE_ARGS="--profile $PROFILE"
    ;;
  *)
    echo "unsupported launcher profile: $PROFILE" >&2
    exit 1
    ;;
esac
APP="$ROOT/dist/$APP_NAME.app"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RESOURCES="$CONTENTS/Resources"
ICON_SVG="$ROOT/packaging/macos/AppIcon.svg"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "$APP_NAME launcher is only supported on macOS" >&2
  exit 1
fi
if [ ! -x "$BIN_DIR/mctrl" ] || [ ! -x "$BIN_DIR/mctrl-runner" ]; then
  echo "build $BIN_DIR/mctrl and $BIN_DIR/mctrl-runner before creating the launcher" >&2
  exit 1
fi
if [ ! -f "$ICON_SVG" ]; then
  echo "missing macOS app icon source: $ICON_SVG" >&2
  exit 1
fi

rm -rf "$APP"
mkdir -p "$MACOS" "$RESOURCES"
cp "$BIN_DIR/mctrl" "$RESOURCES/mctrl"
cp "$BIN_DIR/mctrl-runner" "$RESOURCES/mctrl-runner"
chmod 700 "$RESOURCES/mctrl" "$RESOURCES/mctrl-runner"

ICONSET=$(mktemp -d "${TMPDIR:-/private/tmp}/mctrl-pair.XXXXXX.iconset")
trap 'rm -rf "$ICONSET"' EXIT
sips -s format png -z 1024 1024 "$ICON_SVG" --out "$ICONSET/icon_512x512@2x.png" >/dev/null
sips -z 16 16 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_16x16.png" >/dev/null
sips -z 32 32 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_16x16@2x.png" >/dev/null
sips -z 32 32 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_32x32.png" >/dev/null
sips -z 64 64 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_32x32@2x.png" >/dev/null
sips -z 128 128 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_128x128.png" >/dev/null
sips -z 256 256 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_128x128@2x.png" >/dev/null
sips -z 256 256 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_256x256.png" >/dev/null
sips -z 512 512 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_256x256@2x.png" >/dev/null
sips -z 512 512 "$ICONSET/icon_512x512@2x.png" --out "$ICONSET/icon_512x512.png" >/dev/null
iconutil --convert icns "$ICONSET" --output "$RESOURCES/AppIcon.icns"

cat >"$CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>$APP_NAME</string>
  <key>CFBundleExecutable</key>
  <string>mctrl-pair-launcher</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>$APP_NAME</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$APP_VERSION</string>
  <key>CFBundleVersion</key>
  <string>$APP_VERSION</string>
  <key>LSUIElement</key>
  <true/>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
PLIST

cat >"$MACOS/mctrl-pair-launcher" <<LAUNCHER
#!/bin/zsh
set -u
RESOURCES="\${0:A:h:h}/Resources"
OUTPUT=\$("\$RESOURCES/mctrl" $PROFILE_ARGS pair --open 2>&1)
STATUS=\$?
if [ "\$STATUS" -ne 0 ]; then
  MESSAGE=\$(printf '%s' "\$OUTPUT" | tr '\n' ' ')
  osascript - "\$MESSAGE" <<'APPLESCRIPT' >/dev/null
on run argv
  set messageText to item 1 of argv
  display dialog "Unable to open the $APP_NAME." message messageText buttons {"OK"} default button 1 with icon caution
end run
APPLESCRIPT
fi
exit "\$STATUS"
LAUNCHER
chmod 700 "$MACOS/mctrl-pair-launcher"

codesign --force --sign - --identifier "$BUNDLE_ID" "$APP" >/dev/null
codesign --verify --deep --strict "$APP"
printf '%s\n' "$APP"
