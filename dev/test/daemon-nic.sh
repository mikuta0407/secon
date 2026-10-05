#!/bin/bash
# client-linux で root デーモンの NIC モードを試す (sudo で実行)。
# 使い方: daemon-nic.sh <secon バイナリ> <VPN 接続先 host:port> [default_gateway(true|false)]
source "$(dirname "$0")/lib.sh"
SECON=$1 SERVER=$2 DGW=${3:-false}
DIR=$(mktemp -d)
cat >"$DIR/config.toml" <<EOF
[[profile]]
name = "lab"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "nic"
insecure_skip_verify = true
nic = { routes = ["10.100.0.0/24"], default_gateway = $DGW, dns = true }

[[profile.forward]]
listen = "127.0.0.1:2222"
target = "10.99.0.1:22"
EOF
export SECON_SOCKET=$DIR/secon.sock
# 何があっても 90 秒で終了 (SIGTERM で経路・NIC を片付ける)
(timeout 90 "$SECON" daemon -config "$DIR/config.toml" -socket "$SECON_SOCKET" >"$DIR/daemon.log" 2>&1) &
DPID=$!
sleep 1
echo "== connect"; "$SECON" connect lab
"$SECON" status lab
echo "== link/addr"; ip -br addr show secon-lab
echo "== routes"; ip route | grep -E "secon-lab|^0.0.0.0/1|^128.0.0.0/1|192.168.65.1 "
echo "== HTTP";  curl -sS --max-time 5 http://10.99.0.1/
echo "== static route 10.100.0.1"; curl -sS --max-time 5 http://10.100.0.1/
echo "== SMB";   smbclient -N //10.99.0.1/share -c 'get hello.txt /dev/stdout' 2>/dev/null
echo "== SSH via forward"; timeout 3 nc 127.0.0.1 2222 </dev/null | head -1
echo "== DNS";   resolvectl query target.vpn.test 2>&1 | head -1
if [ "$DGW" = true ]; then
  echo "== route to 1.1.1.1"; ip route get 1.1.1.1 | head -1
  echo "== internet via VPN"; curl -sS --max-time 8 -o /dev/null -w "%{http_code} from %{remote_ip}\n" http://example.com/
fi
echo "== disconnect"; "$SECON" disconnect lab
echo "== after"; ip link show secon-lab 2>&1 | head -1; ip route | grep -cE "secon-lab|^0.0.0.0/1|^128.0.0.0/1" ; resolvectl status 2>/dev/null | grep -c secon-lab
pkill -TERM -f "$SECON daemon -config"; wait $DPID
echo "== daemon log"; cat "$DIR/daemon.log"
