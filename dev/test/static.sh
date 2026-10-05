#!/bin/bash
# 固定アドレス (DHCP を使わない) を NIC / SOCKS 両モードで試す (sudo で実行)。
# 使い方: static.sh <secon バイナリ> <VPN 接続先 host:port>
source "$(dirname "$0")/lib.sh"
SECON=$1 SERVER=$2
DIR=$(mktemp -d)
cat >"$DIR/config.toml" <<EOF
[[profile]]
name = "nic-static"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "nic"
insecure_skip_verify = true
static = { address = "10.99.0.50/24", gateway = "10.99.0.1", dns = ["10.99.0.1"] }
nic = { routes = ["10.100.0.0/24"] }

[[profile]]
name = "socks-static"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "socks"
insecure_skip_verify = true
static = { address = "10.99.0.51/24", gateway = "10.99.0.1", dns = ["10.99.0.1"] }
socks = { listen = "127.0.0.1:1090" }
EOF
export SECON_SOCKET=$DIR/secon.sock
(timeout 90 "$SECON" daemon -config "$DIR/config.toml" -socket "$SECON_SOCKET" >"$DIR/daemon.log" 2>&1) &
DPID=$!
sleep 1
"$SECON" connect nic-static
"$SECON" connect socks-static
"$SECON" status
echo "== NIC: HTTP / static route";  curl -sS --max-time 5 http://10.99.0.1/; curl -sS --max-time 5 http://10.100.0.1/
echo "== SOCKS: HTTP via VPN DNS"; curl -sS --max-time 5 -x socks5h://127.0.0.1:1090 http://target.vpn.test/
"$SECON" disconnect nic-static; "$SECON" disconnect socks-static
pkill -TERM -f "$SECON daemon -config"; wait $DPID
echo "== daemon log"; grep -E "static|dhcp" "$DIR/daemon.log"
