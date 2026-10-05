#!/bin/bash
# 公式 SoftEther クライアントで vpn-server へ接続し、HTTP/SMB/SSH が通るか確認する。
# 使い方 (VM 内 root): official-client-check.sh <host:port>
# 終了時に必ず切断・停止する。
set -uo pipefail
SERVER=$1
vc() { timeout 30 vpncmd localhost /CLIENT /CMD "$@" </dev/null >/dev/null; }

cleanup() {
  vc AccountDisconnect test >/dev/null 2>&1
  dhclient -r vpn_vpn0 >/dev/null 2>&1
  systemctl stop softether-vpnclient
}
trap cleanup EXIT

systemctl start softether-vpnclient
sleep 3
vc AccountDelete test || true
vc NicCreate vpn0 || true
vc AccountCreate test /SERVER:"$SERVER" /HUB:VPN /USERNAME:test /NICNAME:vpn0
vc AccountPasswordSet test /PASSWORD:testpass /TYPE:standard
vc AccountConnect test

for _ in $(seq 20); do
  timeout 20 vpncmd localhost /CLIENT /CMD AccountStatusGet test </dev/null | grep -q "Connection Completed" && break
  sleep 1
done
timeout 20 dhclient -1 vpn_vpn0
ip -br -4 addr show vpn_vpn0

ok=0
curl -fsS --max-time 5 http://10.99.0.1/ && echo "HTTP OK" || ok=1
smbclient -N //10.99.0.1/share -c 'get hello.txt /dev/stdout' 2>/dev/null && echo "SMB OK" || ok=1
nc -z -w 5 10.99.0.1 22 && echo "SSH port OK" || ok=1
exit $ok
