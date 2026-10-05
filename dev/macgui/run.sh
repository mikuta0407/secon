#!/bin/bash
# client-mac のログイン中 GUI セッションでコマンドを実行する (ssh からはウィンドウサーバに届かないため)。
# 使い方: run.sh <command> [args...]   例: macgui.sh screencapture -x /tmp/s.png
exec sudo launchctl asuser "$(id -u)" sudo -u "$(id -un)" "$@"
