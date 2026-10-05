#!/bin/bash
# vpn-server VM のセットアップ (冪等)。VM 内で root 実行する。
#
# VPN 側セグメント: 10.99.0.0/24
#   10.99.0.1   tap_soft (SoftEther Local Bridge) … DHCP/DNS/SSH/HTTP/SMB
#   10.99.0.100-200  DHCP で配布
#   10.100.0.1  サーバの lo (静的ルートのテスト用。10.99.0.1 経由で届く)
# HTTP Proxy: squid :3128 (lima0 側)
set -euo pipefail

HUB=VPN
ADMIN_PASS=adminpass
VPN_USER=test
VPN_PASS=testpass
SEG_IP=10.99.0.1

export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq softether-vpnserver dnsmasq nginx samba squid tcpdump >/dev/null

vpncmd() { timeout 30 /usr/bin/vpncmd localhost:443 /SERVER /PASSWORD:"$ADMIN_PASS" /CMD "$@" </dev/null >/dev/null; }
hubcmd() { timeout 30 /usr/bin/vpncmd localhost:443 /SERVER /PASSWORD:"$ADMIN_PASS" /ADMINHUB:"$HUB" /CMD "$@" </dev/null >/dev/null; }

# 管理パスワード (未設定時のみ)。vpncmd は stdin が無いと空回りするので必ず </dev/null
if ! vpncmd About; then
  timeout 30 /usr/bin/vpncmd localhost:443 /SERVER /CMD ServerPasswordSet "$ADMIN_PASS" </dev/null >/dev/null
fi

vpncmd HubCreate "$HUB" /PASSWORD:"" || true
hubcmd UserCreate "$VPN_USER" /GROUP:none /REALNAME:none /NOTE:none || true
hubcmd UserPasswordSet "$VPN_USER" /PASSWORD:"$VPN_PASS"
vpncmd BridgeCreate "$HUB" /DEVICE:soft /TAP:yes || true

# tap_soft が出来るのを待って IP を振る
for _ in $(seq 30); do ip link show tap_soft >/dev/null 2>&1 && break; sleep 1; done
ip link set tap_soft up
ip addr replace "$SEG_IP/24" dev tap_soft

# 静的ルートのテスト用に、VPN セグメントの外側のアドレスを持たせる
ip addr replace 10.100.0.1/32 dev lo

# 起動時に tap_soft へ IP を振り直す
cat >/etc/systemd/system/vpnseg.service <<EOF
[Unit]
After=softether-vpnserver.service
Requires=softether-vpnserver.service
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/bash -c 'for i in \$(seq 30); do ip link show tap_soft && break; sleep 1; done; ip link set tap_soft up; ip addr replace $SEG_IP/24 dev tap_soft; ip addr replace 10.100.0.1/32 dev lo'
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable -q vpnseg.service

# NAT (VPN をデフォルトGWにしたときの出口)
sysctl -qw net.ipv4.ip_forward=1
echo net.ipv4.ip_forward=1 >/etc/sysctl.d/90-vpnseg.conf
iptables -t nat -C POSTROUTING -s 10.99.0.0/24 -o lima0 -j MASQUERADE 2>/dev/null ||
  iptables -t nat -A POSTROUTING -s 10.99.0.0/24 -o lima0 -j MASQUERADE

# DHCP / DNS
cat >/etc/dnsmasq.d/vpnseg.conf <<EOF
interface=tap_soft
bind-dynamic
dhcp-range=10.99.0.100,10.99.0.200,255.255.255.0,1h
dhcp-option=option:router,$SEG_IP
dhcp-option=option:dns-server,$SEG_IP
domain=vpn.test
address=/target.vpn.test/$SEG_IP
# 上流は systemd-resolved (Ubuntu の dnsmasq は resolv.conf をそのままでは使わない)
no-resolv
server=127.0.0.53
EOF
systemctl restart dnsmasq

# HTTP
echo "hello from vpn-server" >/var/www/html/index.html

# SMB
mkdir -p /srv/share && echo "smb test file" >/srv/share/hello.txt && chmod -R a+rX /srv/share
grep -q '^\[share\]' /etc/samba/smb.conf || cat >>/etc/samba/smb.conf <<EOF
[share]
   path = /srv/share
   guest ok = yes
   read only = yes
EOF
id smbtest >/dev/null 2>&1 || useradd -M -s /usr/sbin/nologin smbtest
printf 'smbpass\nsmbpass\n' | smbpasswd -a -s smbtest >/dev/null
systemctl restart smbd

# HTTP Proxy
cat >/etc/squid/conf.d/vpntest.conf <<EOF
acl localnet src 192.168.0.0/16
http_access allow localnet
EOF
systemctl restart squid

echo "server setup done"
