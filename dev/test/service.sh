#!/bin/bash
# システムサービスとして登録し、一般ユーザから CLI で操作できるか試す。
# 使い方 (一般ユーザで実行、内部で sudo を使う): service.sh <secon バイナリ> <VPN 接続先>
source "$(dirname "$0")/lib.sh"
SECON=$1 SERVER=$2
# デーモンは "secon" グループがあればそれにソケットを開放する。テスト用に作って自分を入れ、sg で使う
sudo groupadd -f secon 2>/dev/null || sudo dseditgroup -o create secon 2>/dev/null
if [ "$(uname)" = Linux ]; then
  sudo usermod -aG secon "$(id -un)"
  secon() { sg secon -c "/usr/local/bin/secon $*"; }
else
  secon() { /usr/local/bin/secon "$@"; }  # macOS は admin グループ
fi
sudo install -m 755 "$SECON" /usr/local/bin/secon
sudo rm -f /etc/secon/config.toml
echo "== install"; sudo /usr/local/bin/secon service install -config /etc/secon/config.toml
sudo tee /etc/secon/config.toml >/dev/null <<EOF
[[profile]]
name = "lab"
server = "$SERVER"
hub = "VPN"
user = "test"
password = "testpass"
mode = "socks"
auto_connect = true
insecure_skip_verify = true
EOF
sleep 1
echo "== reload (as $(id -un))"; secon reload
for i in $(seq 20); do secon status | grep -q connected && break; sleep 0.5; done
secon status
echo "== use"; curl -s -x socks5h://127.0.0.1:1080 http://target.vpn.test/
echo "== socket perms"; ls -l /var/run/secon.sock
echo "== uninstall"; sudo /usr/local/bin/secon service uninstall
sleep 1; secon status 2>&1 | head -1
