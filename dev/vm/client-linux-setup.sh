#!/bin/bash
# client-linux VM のセットアップ (冪等)。VM 内で root 実行する。
# 公式 SoftEther クライアント (比較・基準確認用) とテストツールを入れる。
# 公式クライアントは普段は停止しておく (secon と競合させない)。
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq softether-vpnclient isc-dhcp-client smbclient curl tcpdump netcat-openbsd >/dev/null
systemctl disable -q --now softether-vpnclient || true
echo "client-linux setup done"
