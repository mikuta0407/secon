#!/bin/bash
# macOS VM で GUI を試すためのテスト用ビルドを作る (本番ビルドには使わない)。
#
# VM (Virtualization.framework) にはハードウェア描画の OpenGL が無く、GLFW が
# NSOpenGLPFAAccelerated を要求するため Fyne が起動しない。GLFW のコピーからその要求を外し、
# 別の go.mod (-modfile) で差し替えてビルドする。リポジトリの go.mod は変更しない。
#
# 使い方: dev/macgui/build.sh <出力ディレクトリ>   → secon-gui-vm, click, wins を作る
set -euo pipefail
OUT=$(cd "${1:?output dir}" && pwd)
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
MOD=$(go list -m -f '{{.Dir}}' github.com/go-gl/glfw/v3.4/glfw)
rm -rf "$OUT/glfw-sw" && cp -R "$MOD" "$OUT/glfw-sw" && chmod -R u+w "$OUT/glfw-sw"
sed -i '' 's|^    ADD_ATTRIB(NSOpenGLPFAAccelerated);|    // VM テスト用: Accelerated を要求しない|' "$OUT/glfw-sw/glfw/src/nsgl_context.m"
cp go.mod "$OUT/go.vmtest.mod" && cp go.sum "$OUT/go.vmtest.sum"
echo "replace github.com/go-gl/glfw/v3.4/glfw => $OUT/glfw-sw" >>"$OUT/go.vmtest.mod"
go build -modfile="$OUT/go.vmtest.mod" -o "$OUT/secon-gui-vm" ./cmd/secon-gui
clang -framework ApplicationServices -o "$OUT/click" dev/macgui/click.c
clang -framework ApplicationServices -o "$OUT/wins" dev/macgui/wins.c
echo "built: $OUT/secon-gui-vm $OUT/click $OUT/wins"
