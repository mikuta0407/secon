# 開発環境

ホストのネットワークを壊さないよう、secon の実行・試験はすべて VM 内で行う。

| VM | 種類 | 役割 |
|---|---|---|
| `vpn-server` | Lima (Ubuntu 24.04) | SoftEther Server 5.01 (hub `VPN`, user `test`/`testpass`)。VPN 側セグメント 10.99.0.0/24 に DHCP/DNS (`vpn.test`)・HTTP・SMB・SSH、10.100.0.1 (静的ルート試験用)、squid :3128 |
| `client-linux` | Lima (Ubuntu 24.04) | Linux 版の試験 (公式クライアントも比較用に入っている) |
| `client-mac` | Tart (macOS 26) | macOS 版の試験 (admin/admin, パスワード無し sudo) |

vmnet の NAT は VM 同士の通信を遮断する (ブリッジメンバーが PRIVATE) ため、ホスト上の `dev/relay` が
`<ホストのブリッジIP>:8443 → vpn-server:443`、`:3128 → squid` を中継する。

```sh
dev/vm/up.sh                 # VM 起動 + 中継起動。接続先が表示される
dev/vm/mac-ssh-setup.sh      # 初回のみ: client-mac に SSH 鍵を登録
source dev/vm/env.sh         # mac_ssh / mac_scp / lima_ip などのヘルパー
```

初回構築: `dev/vm/server-setup.sh` / `dev/vm/client-linux-setup.sh` を各 VM 内で root 実行 (冪等)。

## テスト (VM 内で実行)

| スクリプト | 内容 |
|---|---|
| `dev/test/daemon-socks.sh` | ユーザデーモン + CLI、SOCKS5 / ポートフォワード / 認証エラー |
| `dev/test/daemon-nic.sh` | Linux NIC モード (sudo)。第 3 引数 `true` でデフォルト GW 化も |
| `dev/test/daemon-nic-mac.sh` | macOS NIC モード (sudo)。同上 |
| `dev/test/service.sh` | systemd / launchd 登録と一般ユーザからの操作 |
| `dev/test/gui-linux.sh` | Xvfb 上で GUI を起動し、xdotool でプロファイル追加→スクリーンショット (要: client-linux で GUI をビルド) |
| `dev/vm/official-client-check.sh` | 公式クライアントでサーバ環境自体を確認 |

デーモンはテスト中も `timeout` で必ず終了し、SIGTERM で経路・NIC・DNS を片付ける。
