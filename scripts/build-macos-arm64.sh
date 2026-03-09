#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME="Koko Tools"
BINARY_NAME="koko-tools"
DIST_DIR="dist/macos-arm64"
BUNDLE_DIR="$DIST_DIR/$APP_NAME.app"
CONTENTS_DIR="$BUNDLE_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
ICONSET_DIR="$DIST_DIR/koko-tools.iconset"
ICON_PNG="assets/koko-tools-icon-120.png"
ICON_SVG="assets/koko-tools-icon.svg"
ICON_ICNS="$RESOURCES_DIR/koko-tools.icns"
ASSETS_DIR="assets"

build_bundle=false
if [[ "${1:-}" == "--bundle" ]]; then
  build_bundle=true
fi

if ! command -v brew >/dev/null 2>&1; then
  echo "Homebrew is required on macOS Apple Silicon." >&2
  exit 1
fi

if ! command -v pkg-config >/dev/null 2>&1 && ! command -v pkgconf >/dev/null 2>&1; then
  echo "pkg-config or pkgconf is required." >&2
  exit 1
fi

export PKG_CONFIG_PATH="$(brew --prefix)/lib/pkgconfig:$(brew --prefix)/share/pkgconfig:${PKG_CONFIG_PATH:-}"
export CGO_ENABLED=1

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

go build -o "$DIST_DIR/$BINARY_NAME" .
mkdir -p "$DIST_DIR/assets"
cp -R "$ASSETS_DIR/." "$DIST_DIR/assets/"

if [[ "$build_bundle" == false ]]; then
  echo "Built $DIST_DIR/$BINARY_NAME"
  exit 0
fi

mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"
cp "$DIST_DIR/$BINARY_NAME" "$MACOS_DIR/$BINARY_NAME"
mkdir -p "$RESOURCES_DIR/assets"
cp -R "$ASSETS_DIR/." "$RESOURCES_DIR/assets/"

cat > "$CONTENTS_DIR/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>Koko Tools</string>
  <key>CFBundleDisplayName</key>
  <string>Koko Tools</string>
  <key>CFBundleExecutable</key>
  <string>koko-tools</string>
  <key>CFBundleIdentifier</key>
  <string>com.github.kloneets.tools</string>
  <key>CFBundleVersion</key>
  <string>0.0.1</string>
  <key>CFBundleShortVersionString</key>
  <string>0.0.1</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleIconFile</key>
  <string>koko-tools</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
PLIST

if command -v sips >/dev/null 2>&1 && command -v iconutil >/dev/null 2>&1; then
  mkdir -p "$ICONSET_DIR"
  cp "$ICON_PNG" "$ICONSET_DIR/icon_128x128.png"
  sips -z 16 16 "$ICON_PNG" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null
  sips -z 32 32 "$ICON_PNG" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null
  sips -z 32 32 "$ICON_PNG" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null
  sips -z 64 64 "$ICON_PNG" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null
  sips -z 128 128 "$ICON_PNG" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null
  sips -z 256 256 "$ICON_PNG" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null
  sips -z 512 512 "$ICON_PNG" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null
  sips -z 512 512 "$ICON_PNG" --out "$ICONSET_DIR/icon_512x512.png" >/dev/null
  cp "$ICONSET_DIR/icon_512x512.png" "$ICONSET_DIR/icon_512x512@2x.png"
  iconutil -c icns "$ICONSET_DIR" -o "$ICON_ICNS"
  rm -rf "$ICONSET_DIR"
fi

echo "Built $BUNDLE_DIR"
