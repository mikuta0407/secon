# 開発用 VM 環境の共通設定。他スクリプトから source する。
DEV_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STATE_DIR=${SECON_DEV_STATE:-$HOME/.cache/secon-dev}
mkdir -p "$STATE_DIR"

# Lima VM の vzNAT 側 IPv4 (lima0)
lima_ip() { limactl shell "$1" ip -4 -o addr show lima0 | awk '{print $4}' | cut -d/ -f1; }

# vzNAT 側のホスト (ゲートウェイ) アドレス
host_ip() { limactl shell "$1" ip -4 route show dev lima0 default | awk '{print $3}'; }

# クライアント VM から見た VPN サーバ / HTTP Proxy の宛先 (ホストの中継経由)
RELAY_VPN_PORT=8443
RELAY_PROXY_PORT=3128

# macOS VM (Tart) への SSH。鍵は初回に dev/vm/mac-ssh-setup.sh が登録する
MAC_SSH_KEY=$STATE_DIR/id_ed25519
mac_ssh() { ssh -i "$MAC_SSH_KEY" -o UserKnownHostsFile="$STATE_DIR/known_hosts" -o StrictHostKeyChecking=accept-new -o BatchMode=yes "admin@$(tart ip client-mac)" "$@"; }
mac_scp() { scp -q -i "$MAC_SSH_KEY" -o UserKnownHostsFile="$STATE_DIR/known_hosts" -o BatchMode=yes "$1" "admin@$(tart ip client-mac):$2"; }
