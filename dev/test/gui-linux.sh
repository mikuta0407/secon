#!/bin/bash
# client-linux で Xvfb 上に secon-gui を起動し、設定画面のスクリーンショットを撮る。
# 使い方: gui-linux.sh <secon> <secon-gui> <VPN 接続先> <出力 png>
source "$(dirname "$0")/lib.sh"
SECON=$1 GUI=$2 SERVER=$3 OUT=$4
DIR=$(mktemp -d)
cat >"$DIR/config.toml" <<EOF
[[profile]]
name = "lab"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "socks"
auto_connect = true
insecure_skip_verify = true

[[profile.forward]]
listen = "127.0.0.1:2222"
target = "10.99.0.1:22"
EOF
export SECON_SOCKET=$DIR/secon.sock
(timeout 120 "$SECON" daemon -config "$DIR/config.toml" -socket "$SECON_SOCKET" >"$DIR/daemon.log" 2>&1) &
Xvfb :99 -screen 0 1280x800x24 >/dev/null 2>&1 &
XPID=$!
sleep 1
export DISPLAY=:99 LIBGL_ALWAYS_SOFTWARE=1
# 新規プロファイルを GUI から作って保存する (座標は 1280x800 で設定画面が左上に出る前提)
dbus-launch --exit-with-session bash -c "
  (timeout 60 '$GUI' -settings >'$DIR/gui.log' 2>&1 &); sleep 6
  click() { xdotool mousemove \$1 \$2 click 1; sleep 0.3; }
  typ() { click \$1 \$2; xdotool key ctrl+a; xdotool type --delay 20 \"\$3\"; }
  click 97 618
  typ 530 21 lab2; typ 530 60 '$SERVER'; typ 530 138 test; typ 510 177 testpass
  click 320 334
  typ 530 412 127.0.0.1:1081
  click 734 618; sleep 4
  import -window root '$OUT'"
echo "== config after GUI save"; grep -A8 'name = "lab2"' "$DIR/config.toml"
echo "== status"; "$SECON" status
echo "== gui log"; cat "$DIR/gui.log"
pkill -x "$(basename "$GUI")"; pkill -TERM -f "$SECON daemon -config"; kill $XPID
echo "== daemon log"; cat "$DIR/daemon.log"
