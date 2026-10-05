#!/bin/bash
# macOS で NIC モード (default_gateway + DNS) 接続中にデーモンを強制終了し、再起動時の後片付けと再接続を試す (sudo)。
# 使い方: crash-mac.sh <secon バイナリ> <VPN 接続先>
source "$(dirname "$0")/lib.sh"
SECON=$1 SERVER=$2
DIR=$(mktemp -d)
cat >"$DIR/config.toml" <<EOF
[[profile]]
name = "crash"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "nic"
insecure_skip_verify = true
auto_connect = true
nic = { default_gateway = true, dns = true }
EOF
export SECON_SOCKET=$DIR/secon.sock
"$SECON" daemon -config "$DIR/config.toml" -socket "$SECON_SOCKET" >"$DIR/d1.log" 2>&1 &
D1=$!
for i in $(seq 30); do "$SECON" status 2>/dev/null | grep -q connected && break; sleep 0.5; done
echo "== connected, DNS keys:"; scutil <<<"list State:/Network/Service/secon-.*" | grep -c secon
kill -9 $D1; sleep 1
echo "== after kill -9, DNS keys left:"; scutil <<<"list State:/Network/Service/secon-.*" | grep -c secon
rm -f "$SECON_SOCKET"
(timeout 40 "$SECON" daemon -config "$DIR/config.toml" -socket "$SECON_SOCKET" >"$DIR/d2.log" 2>&1) &
for i in $(seq 40); do "$SECON" status 2>/dev/null | grep -q connected && break; sleep 0.5; done
echo "== restarted:"; "$SECON" status | tail -1
echo "== internet via VPN:"; curl -sS --max-time 8 -o /dev/null -w "%{http_code}\n" http://example.com/
pkill -TERM -f "$SECON daemon -config"; sleep 2
echo "== after clean stop, DNS keys left:"; scutil <<<"list State:/Network/Service/secon-.*" | grep -c secon
echo "== restart log:"; grep -E "stale|route|established" "$DIR/d2.log"
