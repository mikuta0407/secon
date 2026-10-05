#!/bin/bash
# secon.app を組み立てる。
# 使い方: build-app.sh <secon-gui バイナリ> <secon バイナリ> <バージョン> <出力ディレクトリ>
#   → <出力ディレクトリ>/secon.app
# secon (CLI) も同梱する (Homebrew Cask がここから bin にリンクする)。
set -euo pipefail
GUI=$1 CLI=$2 VERSION=$3 OUT=$4
HERE=$(cd "$(dirname "$0")" && pwd)
APP="$OUT/secon.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
install -m 755 "$GUI" "$APP/Contents/MacOS/secon-gui"
install -m 755 "$CLI" "$APP/Contents/MacOS/secon"
cp "$HERE/secon.icns" "$APP/Contents/Resources/secon.icns"
sed "s/@VERSION@/$VERSION/g" "$HERE/Info.plist.in" > "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist" >/dev/null
# ad-hoc 署名 (Apple Silicon では未署名のバイナリは起動できない。バンドル全体として署名し直す)
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict "$APP"
echo "$APP"
