#!/bin/bash
# client-linux でユーザ権限のデーモン + CLI (SOCKS モード) を試す。
# 使い方: daemon-socks.sh <secon バイナリ> <VPN 接続先 host:port>
source "$(dirname "$0")/lib.sh"
SECON=$1 SERVER=$2
CFG=$(mktemp -d)/config.toml
cat >"$CFG" <<EOF
[[profile]]
name = "lab"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "socks"
insecure_skip_verify = true

[[profile.forward]]
listen = "127.0.0.1:2222"
target = "10.99.0.1:22"

[[profile]]
name = "badpass"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "nope"
insecure_skip_verify = true
socks = { listen = "127.0.0.1:1081" }
EOF
export SECON_SOCKET=$(dirname "$CFG")/secon.sock
(timeout 100 "$SECON" daemon -config "$CFG" -socket "$SECON_SOCKET" >"$(dirname "$CFG")/daemon.log" 2>&1) &
DPID=$!
sleep 1
echo "== status"; "$SECON" status
echo "== connect lab"; "$SECON" connect lab
echo "== connect badpass"; "$SECON" connect badpass
echo "== status"; "$SECON" status
echo "== detail"; "$SECON" status lab
echo "== use"; curl -s -x socks5h://127.0.0.1:1080 http://target.vpn.test/; timeout 3 nc 127.0.0.1 2222 </dev/null | head -1
echo "== disconnect"; "$SECON" disconnect lab; "$SECON" disconnect badpass; "$SECON" status
curl -s --max-time 3 -x socks5h://127.0.0.1:1080 http://target.vpn.test/ || echo "(socks closed as expected)"
pkill -TERM -f "$SECON daemon -config"; wait $DPID
echo "== daemon log"; cat "$(dirname "$CFG")/daemon.log"
