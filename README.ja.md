# secon

```sh
brew install mikuta0407/apps/secon
```

Go で書いた、macOS / Linux 向けの SoftEther VPN 互換クライアントです。
仮想 NIC として接続するか、アプリ内で終端して SOCKS5 プロキシとして使えます。

[English README](README.md)

![接続マネージャ](docs/images/manager.png)

## 5 分で使い始める

1. インストール: `brew install mikuta0407/apps/secon`
2. デーモンを登録: `sudo secon service install`
3. 接続設定を追加: `secon-gui` を起動 → トレイの **接続マネージャ…** → **新規**
4. 接続: **接続** ボタン、または `secon connect <接続設定名>`
5. 確認: `secon status <接続設定名>` で IP アドレス・ゲートウェイ・通信量が見られます

GUI を使わない場合は、手順 3 で `/etc/secon/config.toml` を編集して（[設定ファイル](#設定ファイル)）`secon reload` を実行します。

## できること

| 機能 | 内容 |
|---|---|
| NIC モード | 仮想 NIC（Linux: TAP / macOS: utun）を作り、DHCP でアドレスを取得。普通の L2 VPN と同じように使えます（SSH・HTTP・SMB で確認済み） |
| SOCKS5 モード | 仮想 NIC を作らず root も不要。TCP/IP を secon 内で処理し、SOCKS5 プロキシとして待ち受けます |
| ポート転送 | ローカルのポートで待ち受け、VPN の向こうのホストへ転送。両モードで使えます（RDP クライアントなど SOCKS 非対応のアプリ用） |
| ルーティング | VPN 側への静的経路の追加、VPN のデフォルトゲートウェイ化、VPN 側 DNS の設定 |
| HTTP Proxy 通過 | HTTP プロキシ経由（`CONNECT`）で VPN サーバに接続 |

### どちらのモードを使う?

| 使い方 | 選ぶモード |
|---|---|
| すべてのアプリから VPN の先に届かせたい | `nic`（root デーモンが必要） |
| ブラウザなど SOCKS5 対応のアプリだけでよい | `socks`（root 不要・OS の経路を変えない） |
| RDP など SOCKS 非対応のアプリを 1 つだけ通したい | どちらのモードでも可 + ポート転送 |

## GUI の使い方

`secon-gui` はメニューバー（macOS）/ システムトレイ（Linux）に常駐します。操作はすべてデーモン経由なので、先にデーモンを登録してください（`sudo secon service install`）。

- macOS: Homebrew で `secon` と一緒に入ります
- Linux: ソースからビルドします（[開発](#開発)）。トレイの表示には StatusNotifierItem 対応のパネルが必要です（GNOME なら AppIndicator 拡張）

![トレイメニュー](docs/images/tray.png)

### 接続・切断する

1. トレイのアイコンをクリック（緑 = どれかが接続中）
2. 接続設定にカーソルを合わせる
3. **接続** または **切断** をクリック

### 接続マネージャ

トレイメニュー → **接続マネージャ…**

| 場所 | 表示・操作 |
|---|---|
| ツールバー | 選択中の接続設定の 接続 / 切断 / 新規 / プロパティ / 削除 |
| 一覧 | 状態・モード・接続先・仮想 HUB・IP アドレス・受信・送信・接続時間（2 秒ごとに更新） |
| 下部 | ゲートウェイ・DNS・仮想 NIC・SOCKS5・ポート転送・セッション・サーバのビルド・パケット数 |

### 接続設定を作る・編集する

1. 接続マネージャで **新規**、または接続設定を選んで **プロパティ**
2. サーバ（`ホスト:ポート`）・仮想 HUB・ユーザ名・パスワードを入力（パスワードが空なら匿名認証）
3. モードを選ぶ。そのモードで使う項目だけが表示されます
4. **保存**。デーモンが設定ファイルを書き換え、すぐに反映します

![接続設定のプロパティ](docs/images/properties.png)

既存の接続設定のプロパティには、接続 / 切断ボタンと **接続情報** タブもあります。

## CLI の使い方

| コマンド | 内容 |
|---|---|
| `secon status [接続設定名]` | 一覧、または 1 件の詳細（アドレス・ゲートウェイ・DNS・通信量） |
| `secon connect <接続設定名>` | 接続し、完了まで待つ（最大 60 秒） |
| `secon disconnect <接続設定名>` | 切断 |
| `secon reload` | 設定ファイルを読み直す（`SIGHUP` と同じ） |
| `secon service install` / `uninstall` | デーモンを登録 / 解除（launchd・systemd） |

そのほか: `secon version`、`secon daemon`（前面で起動）、`secon debug dump|socks`（デーモンを介さず直接接続。調査用）

```console
$ secon status
NAME          MODE   STATE         ADDRESS         SERVER
office        nic    connected     10.99.0.137/24  vpn.example.com:443
office-socks  socks  connected     10.99.0.190/24  vpn.example.com:443
datacenter    nic    disconnected  -               vpn.example.com:443
```

### デーモンと権限

| 登録方法 | コマンド | 使えるモード |
|---|---|---|
| システムデーモン（推奨） | `sudo secon service install` | NIC・SOCKS |
| ユーザデーモン | `secon service install --user` | SOCKS のみ |

- macOS: `admin` グループのユーザは `sudo` なしで `secon` を操作できます
- Linux: `secon` → `sudo` → `wheel` のうち最初に存在するグループ。専用グループにする場合は `sudo groupadd secon && sudo usermod -aG secon $USER` のあと再ログイン
- ログ: `/var/log/secon.log`（macOS）、`journalctl -u secon`（Linux）。macOS のユーザデーモンは `~/Library/Logs/secon.log`

## 設定ファイル

システムデーモンは `/etc/secon/config.toml`。ユーザデーモンは `~/.config/secon/config.toml`（Linux）/ `~/Library/Application Support/secon/config.toml`（macOS）。`secon service install` がコメント付きのひな形を作ります。

```toml
[[profile]]
name = "office"
server = "vpn.example.com:443"
hub = "VPN"
user = "alice"
password = "secret"          # 空なら匿名認証
mode = "nic"                 # "nic" または "socks"
auto_connect = true
cert_sha256 = "0e84ded0..."  # 自己署名のサーバ証明書をピン留め
# proxy = "http://user:pass@proxy.example.com:8080"

[profile.nic]
routes = ["10.20.0.0/16"]    # このネットワークだけ VPN 経由にする
default_gateway = false      # true ならすべての通信を VPN 経由にする
dns = true                   # VPN 側の DNS を使う
dns_domains = ["corp.example"]

[[profile.forward]]
listen = "127.0.0.1:13389"
target = "10.20.0.5:3389"
```

SOCKS5 の接続設定は `mode = "socks"` にして、`[profile.socks]` に `listen = "127.0.0.1:1080"`（必要なら `username` / `password`）を書きます。

### サーバ証明書

1. 既定: OS の信頼ストアで検証
2. 自己署名: `secon debug dump -server vpn.example.com:443 -insecure -duration 1s` を実行し、表示される `cert sha256=` の値を `cert_sha256` に書く（その後の認証エラーは無視して構いません）
3. `insecure_skip_verify = true` で検証しない（テスト用）

### 経路とデフォルトゲートウェイ（NIC モード）

- DHCP でもらったサブネットは設定なしで届きます
- `routes`: VPN 側ルータのさらに奥のネットワーク。DHCP のルータへ送り、それ以外は普段の回線のまま
- `default_gateway = true`: すべての通信を VPN 経由に。VPN サーバ自体への通信だけは元の経路を使います
- 経路と DNS の設定は切断時に元に戻ります

## 制限事項

- 認証はパスワードと匿名のみ（証明書認証・RADIUS は未対応）
- セッションあたり TCP 1 本。UDP 高速化なし
- IPv4 のみ
- 動作確認したサーバは SoftEther VPN Server 5.01
- GUI の表示は日本語のみ

## アンインストール

1. `sudo secon service uninstall`
2. `brew uninstall secon`
3. 必要なら `sudo rm -r /etc/secon`

## 開発

- `docs/DESIGN.md`: 設計
- `docs/PROTOCOL.md`: SoftEther プロトコルのメモ
- `dev/README.md`: VM（Lima + Tart）を使ったテスト環境

ビルド: `go build ./cmd/secon`（Go のみ）、`go build ./cmd/secon-gui`（cgo が必要。Linux では X11 / OpenGL のヘッダも必要）

## ライセンス

Apache License 2.0。[LICENSE](LICENSE) と [NOTICE](NOTICE) を参照してください。

secon は SoftEther VPN プロトコルの独立した実装で、SoftEther VPN のソースコードは含みません。SoftEther VPN Project とは関係ありません。
