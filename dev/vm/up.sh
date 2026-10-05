#!/bin/bash
# 開発用 VM を起動し、ホスト上の中継 (dev/relay) を立ち上げる。
#
# vmnet (vzNAT) はブリッジの各 VM に PRIVATE が付いて VM 同士が直接通信できないため、
# クライアント VM → ホスト:8443 → vpn-server:443 と中継する。
set -euo pipefail
source "$(dirname "$0")/env.sh"

for vm in vpn-server client-linux; do
  limactl list --format '{{.Status}}' "$vm" | grep -q Running || limactl start --tty=false "$vm"
done
if tart list --quiet 2>/dev/null | grep -qx client-mac && ! tart ip client-mac >/dev/null 2>&1; then
  nohup tart run --vnc-experimental client-mac >"$STATE_DIR/client-mac.log" 2>&1 &
fi

SERVER=$(lima_ip vpn-server)
HOST=$(host_ip vpn-server)

if [ -f "$STATE_DIR/relay.pid" ] && kill -0 "$(cat "$STATE_DIR/relay.pid")" 2>/dev/null; then
  kill "$(cat "$STATE_DIR/relay.pid")"
fi
go build -o "$STATE_DIR/relay" "$DEV_DIR/relay"
nohup "$STATE_DIR/relay" \
  -map "$HOST:$RELAY_VPN_PORT=$SERVER:443" \
  -map "$HOST:$RELAY_PROXY_PORT=$SERVER:3128" \
  >"$STATE_DIR/relay.log" 2>&1 &
echo $! >"$STATE_DIR/relay.pid"

echo "vpn-server : $SERVER"
echo "VPN 接続先 : $HOST:$RELAY_VPN_PORT  (hub=VPN user=test pass=testpass)"
echo "HTTP Proxy : http://$HOST:$RELAY_PROXY_PORT"
