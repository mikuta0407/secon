# secon 設計メモ (v0)

Linux / macOS 向け SoftEther VPN クライアント (Go)。
本書は「方針と骨組み」だけを決める。詳細は実装しながら詰める。

---

## 1. 全体像

```
 ┌──────────── secon-gui (メニューバー / トレイ) ───┐   ┌── secon (CLI) ──┐
 └────────────────────┬────────────────────────────┘   └────────┬────────┘
                      │  Unix socket (HTTP+JSON)                  │
                      ▼                                           ▼
 ┌────────────────────────── secon daemon (常駐デーモン) ──────────────────────────┐
 │  Profile Manager ── 接続ごとに Session を1つ持つ                               │
 │                                                                                │
 │  Session                                                                       │
 │   ├─ Transport : TCP → (HTTP Proxy CONNECT) → TLS                              │
 │   ├─ Protocol  : PACK / Hello / Auth / データチャネル (Ethernetフレーム送受信)  │
 │   └─ Link      : フレームの行き先 (どちらか一方)                               │
 │        ├─ [NIC モード]   OS仮想NIC (Linux: TAP / macOS: utun + L2エミュ)       │
 │        │                 + Route / DNS 設定                                    │
 │        └─ [SOCKS モード] ユーザ空間TCP/IPスタック (gVisor netstack)            │
 │                          + 内蔵DHCPクライアント                                │
 │                                                                                │
 │  Dialer (モード共通の抽象)                                                     │
 │   ├─ SOCKS5 サーバ   … SOCKSモード時のみ                                       │
 │   └─ ポートフォワード … 両モードで利用可                                       │
 └────────────────────────────────────────────────────────────────────────────────┘
```

**最重要の抽象は 2 つだけ:**

```go
// VPNセッションとローカル側をつなぐ L2 フレームの口
type Link interface {
    ReadFrame([]byte) (int, error)   // ローカル → VPN へ送るフレーム
    WriteFrame([]byte) error         // VPN → ローカルへ渡すフレーム
    Close() error
}

// 「VPN の向こう」へ TCP/UDP を張る手段
type Dialer interface {
    DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}
```

- NIC モード: `Dialer` = 普通の `net.Dialer` (OS のルーティングに任せる)
- SOCKS モード: `Dialer` = netstack の `gonet.DialContextTCP` 等

これでポートフォワード / SOCKS5 はモードを意識せずに書ける。

---

## 2. SoftEther プロトコル実装 (最大のリスク)

公式仕様書は無いので **SoftEtherVPN のソース (`src/Cedar/Protocol.c`, `Connection.c`, `src/Mayaqua/Pack.c`) を読んで互換実装**する。

| ステップ | 内容 |
|---|---|
| 1. 接続 | TCP (必要なら HTTP Proxy に `CONNECT host:443`) → TLS |
| 2. Signature | `POST /vpnsvc/connect.cgi` で `VPNCONNECT` 署名を送る |
| 3. Hello | サーバから PACK 受信 (`random` 20byte, バージョン等) |
| 4. Auth | PACK 送信。パスワード認証は `SHA0(password + UPPER(user))` → `SHA0(hash + random)` ※**SHA-0** は Go 標準に無いので自前実装 |
| 5. Welcome | セッション名、max_connection、UDP加速情報などを受信 |
| 6. データ | TLS 上を `[個数 u32][サイズ u32][Ethernetフレーム]...` のブロックで送受信。KeepAlive は個数=`0xFFFFFFFF` |

**初期スコープで割り切るもの**
- TCP コネクション数は 1 本 (max_connection=1)
- UDP 高速化 / R-UDP / 圧縮 は後回し
- 認証は 匿名 / パスワード のみ (証明書認証・RADIUS は後で)
- サーバ証明書検証: 「初回ピン留め (TOFU)」+「検証スキップ」オプション

---

## 3. モード別設計

### 3.1 NIC モード (root 必要)

| | Linux | macOS |
|---|---|---|
| 仮想NIC | `/dev/net/tun` で **TAP** (L2そのまま) | **utun** (L3 のみ。TAP は OS 標準に無い) |
| L2 の扱い | 素通し | デーモン内で L2⇔L3 変換 (ARP 応答/解決、Ethernet ヘッダ付け外し) |
| IP 取得 | 内蔵 DHCP → netlink で設定 (OS の dhclient 等に任せることも可) | 内蔵 DHCP → `ifconfig`/PF_ROUTE で設定 |
| ルート | `vishvananda/netlink` | `golang.org/x/net/route` もしくは `route` コマンド |
| DNS | systemd-resolved (`resolvectl`) | `scutil` で SC Dynamic Store に登録 |

> 「DHCP は OS 側」は Linux TAP なら可能だが、macOS utun では不可。
> **両 OS とも内蔵 DHCP クライアントに統一**するのが実装・挙動ともにシンプル (`insomniacslk/dhcp` でパケット生成)。

### 3.2 SOCKS モード (root 不要)

- `gvisor.dev/gvisor/pkg/tcpip` (netstack) に Ethernet リンクとして `Link` をつなぐ → ARP は netstack が処理
- 内蔵 DHCP でアドレス / GW / DNS を取得し netstack に設定
- SOCKS5 サーバ (CONNECT + UDP ASSOCIATE、認証は none / user-pass)
- 名前解決は DHCP で得た VPN 側 DNS へ netstack 経由で問い合わせ (SOCKS5h 相当にも対応)

### 3.3 ポートフォワード (両モード)

```
listen 127.0.0.1:13389  →  Dialer.Dial("tcp", "10.0.0.5:3389")
```
TCP を基本、UDP は後回し。プロファイルごとに複数定義。

### 3.4 ルーティング (NIC モードのみ)

- `routes: [10.20.0.0/16, 192.168.50.0/24]` を接続後に追加、切断時に削除
- `default_gateway: true` の場合
  1. VPN サーバ (と Proxy) への /32 ホストルートを元の GW 経由で追加
  2. `0.0.0.0/1` + `128.0.0.0/1` を VPN 側 GW へ (元のデフォルトルートを消さない定番手法)
- 追加したルートはデーモンが記録し、異常終了後の起動時にもクリーンアップ

---

## 4. 常駐・管理

### プロセス構成

| バイナリ | 役割 |
|---|---|
| `secon daemon` | デーモン本体。全機能はここ |
| `secon <cmd>` | CLI。デーモンを Unix socket 経由で操作 |
| `secon-gui` | メニューバー/トレイ常駐。同じ API を叩くだけ (cgo が要るので別バイナリ) |

CLI / GUI はデーモンのクライアントにすぎない → ロジックの重複なし。
Homebrew で配りやすいよう、デーモンと CLI は 1 バイナリにまとめた (実装時に変更)。

### API
- Unix socket (`/var/run/secon.sock`、SOCKS のみのユーザ起動時は `$XDG_RUNTIME_DIR` 等) 上の HTTP+JSON
- 状態変化は SSE (or long-poll) で GUI に通知
- 権限: root デーモンは socket を `api.group` (既定: macOS は `admin`、Linux は `secon`→`sudo`→`wheel` の順で存在するもの) に開放

### CLI 例
```
secon status [profile]         # 一覧 / 詳細
secon connect office           # 接続完了まで待つ
secon disconnect office
secon reload                   # 設定ファイル再読み込み (SIGHUP でも可)
secon service install [--user] # launchd / systemd ユニット生成・登録
secon debug dump|socks         # 開発用: デーモンを介さず直接接続
```

### サービス登録
- macOS: `/Library/LaunchDaemons/io.github.mikuta0407.secon.plist` / `--user` で `~/Library/LaunchAgents` (SOCKS のみ)
- Linux: `/etc/systemd/system/secon.service` / `--user` で `systemctl --user` 版

### GUI
- **Fyne** を採用 (`desktop.App.SetSystemTrayMenu` でトレイ、設定画面も同じツールキットで書ける)
- メニュー: プロファイルごとに 接続/切断、状態表示、「設定…」
- 代替案: トレイだけ `fyne.io/systray`、設定画面はデーモンが出すローカル Web UI

---

## 5. 設定ファイル (例: TOML)

```toml
[[profile]]
name        = "office"
server      = "vpn.example.com:443"
hub         = "VPN"
user        = "alice"
password    = "..."             # 将来 Keychain / Secret Service に移行
mode        = "nic"             # "nic" | "socks"
auto_connect = true

proxy       = "http://user:pass@proxy.corp:8080"   # HTTP Proxy 通過
cert_sha256 = "..."             # 自己署名証明書のピン留め (insecure_skip_verify = true で検証なし)

[profile.nic]
default_gateway = false
routes = ["10.20.0.0/16"]
dns = true                      # VPN 側 DNS を OS に設定
dns_domains = ["corp.example"]  # 省略時は DHCP のドメイン名。default_gateway 時は全ドメイン

[profile.socks]
listen = "127.0.0.1:1080"

[[profile.forward]]
listen = "127.0.0.1:13389"
target = "10.20.0.5:3389"
```

置き場所: `/etc/secon/config.toml` (デーモン) — パーミッション 0600。

---

## 6. ディレクトリ構成

```
cmd/secon/       CLI + デーモン (+ 開発用 debug コマンド)
internal/
  proto/         PACK, Hello/Auth, データチャネル, KeepAlive, SHA-0   (仕様: docs/PROTOCOL.md)
  transport/     TCP / HTTP CONNECT / TLS (証明書ピン留め)
  l2/            フレーム上の UDP 組立・解析、DHCP クライアント、utun 用 L2 エミュレータ (ARP)
  usernet/       gVisor netstack 連携 (SOCKS モード)
  nic/           tap_linux / utun_darwin と OS 設定 (netlink・resolvectl / ifconfig・route・scutil)
  socks5/  forward/
  engine/        プロファイルごとの接続ループ・再接続・リスナ、モード (socks / nic)
  api/           Unix socket 上の HTTP+JSON (サーバ / クライアント, SSE)
  config/  service/
dev/             開発用 VM 環境とテストスクリプト (dev/README.md)
```

---

## 7. 進め方 (マイルストーン)

各段階で「動くもの」ができる順番にする。**root 不要な SOCKS モードを先に作る**とプロトコル部分の検証がしやすい。

1. **プロトコル PoC** — 接続・認証して Ethernet フレームを受信、pcap に吐ける (`secon debug dump`)
2. **SOCKS モード** — netstack + DHCP + SOCKS5。curl で VPN 向こうに届けば成功
3. **ポートフォワード**
4. **デーモン + CLI** — Unix socket API、プロファイル管理、launchd/systemd 登録
5. **HTTP Proxy 通過**
6. **NIC モード (Linux TAP)** → **NIC モード (macOS utun + L2 エミュ)**
7. **ルーティング / デフォルト GW / DNS**
8. **GUI**
9. 余力: 再接続強化、複数 TCP、UDP 高速化、証明書認証、Keychain

---

## 8. 主なリスク・未決事項

- **プロトコル互換性**: 非公開仕様。サーバのバージョン差異に注意。現状は SoftEther Server 5.01 (Ubuntu パッケージ) でのみ確認
- **macOS の L2**: 目標は IP 通信 (SSH / HTTP / SMB 等) が通るレベル。ARP・DHCP・ユニキャスト IP を扱えれば十分で、NetBIOS ブロードキャスト等の L2 依存機能は対象外 (決定)
- **配布**: Homebrew (tap + formula) で配布 (決定)。root デーモンは `sudo secon service install` で LaunchDaemon / systemd ユニットを登録する
- **GUI と root デーモンの権限分離**: socket のグループ権限で足りるか
- **パスワード保管**: 当面は平文ファイル (0600)、後で OS のキーストアへ
