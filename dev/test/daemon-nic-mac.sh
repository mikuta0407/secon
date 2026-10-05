#!/bin/bash
# client-mac で root デーモンの NIC モード (utun + L2 エミュレーション) を試す (sudo で実行)。
# 使い方: daemon-nic-mac.sh <secon バイナリ> <VPN 接続先 host:port> [default_gateway(true|false)]
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
EOF
export SECON_SOCKET=$DIR/secon.sock
(timeout 90 "$SECON" daemon -config "$DIR/config.toml" -socket "$SECON_SOCKET" >"$DIR/daemon.log" 2>&1) &
DPID=$!
sleep 1
echo "== connect"; "$SECON" connect lab
IF=$(grep -o 'nic utun[0-9]*' "$DIR/daemon.log" | awk '{print $2}')
echo "== ifconfig $IF"; ifconfig "$IF" | grep -E "inet |mtu"
echo "== routes"; netstat -rn -f inet | grep -E "$IF|^0/1|^128.0/1|192.168.65.1 " 
echo "== ping";  ping -c 2 -t 3 10.99.0.1 | tail -1
echo "== HTTP";  curl -sS --max-time 5 http://10.99.0.1/
echo "== static route 10.100.0.1"; curl -sS --max-time 5 http://10.100.0.1/
echo "== SMB";   mkdir -p "$DIR/mnt"; mount_smbfs -N //guest@10.99.0.1/share "$DIR/mnt" 2>&1 && cat "$DIR/mnt/hello.txt"; umount "$DIR/mnt" 2>/dev/null
echo "== SSH";   nc -w 3 10.99.0.1 22 </dev/null | head -1
echo "== DNS";   dscacheutil -q host -a name target.vpn.test | grep ip_address; scutil --dns | grep -A3 "vpn.test" | head -4
if [ "$DGW" = true ]; then
  echo "== route to 1.1.1.1"; route -n get 1.1.1.1 | grep interface
  echo "== internet via VPN"; curl -sS --max-time 8 -o /dev/null -w "%{http_code} from %{remote_ip}\n" http://example.com/
fi
echo "== disconnect"; "$SECON" disconnect lab
echo "== after"; ifconfig "$IF" 2>&1 | head -1; netstat -rn -f inet | grep -cE "$IF|^0/1|^128.0/1"; scutil --dns | grep -c vpn.test
pkill -TERM -f "$SECON daemon -config"; wait $DPID
echo "== daemon log"; cat "$DIR/daemon.log"
