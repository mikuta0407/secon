# SoftEther VPN クライアントプロトコル仕様メモ (Go 互換実装用)

対象: パスワード認証 (authtype=1) / 匿名認証 (authtype=0)、TCP 1 本、UDP 加速なし、圧縮なし、HTTP CONNECT プロキシ経由あり。
根拠ソース: SoftEtherVPN 本家 master (commit 40120e1, 2026-10-01)。`src/` 以下の相対パスで `ファイル:関数` を示す。
「推測」と書いた箇所以外はソースから直接確認した。

---

## 0. 全体の流れ (要約)

```
TCP 接続 (直 or HTTP CONNECT プロキシ経由)
 → TLS ハンドシェイク (SNI = サーバホスト名)
 → [HTTP] POST /vpnsvc/connect.cgi  (signature: GIF ウォーターマーク or "VPNCONNECT")
 ← [HTTP] 200 OK + PACK(Hello: hello/version/build/random)
 → [HTTP] POST /vpnsvc/vpn.cgi      PACK(method="login", ...)
 ← [HTTP] 200 OK + PACK(Welcome or error)
 ==== ここで HTTP 終了。同じ TLS ストリーム上で生のブロック形式 (§5) に切り替わる ====
```
1 本の TLS 接続上で HTTP/1.1 Keep-Alive のリクエスト/レスポンスが 2 往復し、その直後から生データになる。
(Cedar/Protocol.c:ClientConnect)

---

## 1. 接続手順 (ステップ順)

### 1.1 TCP / プロキシ / TLS
- 直結: `host:port` (既定 443/992/1194/5555 のいずれか) に TCP 接続。
- HTTP プロキシ (Mayaqua/Proxy.c:BindProxyHttpConnect): プロキシへ TCP 接続後、以下を送る。
  ```
  CONNECT <host>:<port> HTTP/1.0\r\n
  User-Agent: Mozilla/5.0 (Windows NT 6.3; WOW64; rv:29.0) Gecko/20100101 Firefox/29.0\r\n
  Host: <host>\r\n
  Content-Length: 0\r\n
  Proxy-Connection: Keep-Alive\r\n
  Pragma: no-cache\r\n
  Proxy-Authorization: Basic base64(user:pass)\r\n   ← user と pass が両方非空のときだけ
  \r\n
  ```
  - IPv6 リテラルは `[addr]:port`。ホスト名中の `/` 以降は切り捨て。
  - 応答ステータス行の 1 トークン目が 8 文字かつ `HTTP/1.` で始まれば 2 トークン目を数値化。2xx で成功、401/403/407 は `ERR_PROXY_AUTH_FAILED` 相当、それ以外は失敗。応答ヘッダは空行まで読み捨て (ボディは読まない)。
  - プロキシ処理のタイムアウトは 4 秒 (Mayaqua/Proxy.h:PROXY_CONNECTION_TIMEOUT)。
- TLS: 上記ソケット上でクライアントとして TLS 開始。SNI = 接続先サーバ名 (Cedar/Protocol.c:ClientConnectToServer → StartSSLEx3(…, c->ServerName, …))。クライアント証明書は送らない (パスワード/匿名では不要)。
  - ハンドシェイク〜Welcome 受信までのソケットタイムアウトは 15 秒 (Cedar/Cedar.h:CONNECTING_TIMEOUT)。
  - サーバ証明書は通常自己署名。本家はユーザ確認 or 登録証明書と比較する (Cedar/Protocol.c:ClientCheckServerCert)。Go では `InsecureSkipVerify` + 自前でフィンガープリント比較が必要。

### 1.2 signature アップロード (Cedar/Protocol.c:ClientUploadSignature)
```
POST /vpnsvc/connect.cgi HTTP/1.1\r\n
Host: <接続先 IP 文字列 (プロキシ経由時はプロキシの IP になる。サーバは見ていない)>\r\n
Content-Type: image/jpeg\r\n
Connection: Keep-Alive\r\n
Content-Length: <ボディ長>\r\n
\r\n
<ボディ>
```
- 本家のボディ: `WaterMark` (Cedar/WaterMark.c、GIF89a で始まる 1411 バイトの固定バイナリ) + 乱数 0〜1999 バイト。
- **サーバ側は代替として、ボディがちょうど ASCII `VPNCONNECT` (10 バイト、NUL なし) でも受理する** (Cedar/Protocol.c:ServerDownloadSignature, Mayaqua/HTTP.h:HTTP_VPN_TARGET_POSTDATA)。Go 実装はこれを使えば WaterMark を埋め込む必要がない。
  - サーバの受理条件: パスが `/vpnsvc/connect.cgi` (大小無視) かつ (`len==10 && body=="VPNCONNECT"` または `1411 <= len <= 1411+2000 && 先頭 1411 バイトが WaterMark 一致`)。
  - 古いサーバ (この代替受理が入る前) では `VPNCONNECT` が通らない可能性あり (推測)。互換重視なら WaterMark を送る。
- このリクエストに対する HTTP レスポンスが、そのまま次の Hello になる (signature 専用の応答はない)。

### 1.3 Hello 受信 (Cedar/Protocol.c:ClientDownloadHello, GetHello / PackHello)
HTTP レスポンス (§3.2 の形) のボディが PACK。
| 名前 | 型 | 内容 |
|---|---|---|
| `hello` | STR | サーバ製品文字列 (例 "SoftEther VPN Server Developer Edition") |
| `version` | INT | サーババージョン (例 502 = 5.02) |
| `build` | INT | サーバビルド (例 5187) |
| `random` | DATA (20 バイト必須) | パスワードハッシュ用チャレンジ |
| `pencore` | DATA | ダミー乱数 (無視) |
- `error` (INT) が非 0 ならそのエラーで終了。`hello` が無い / `random` が 20 バイトでない → `ERR_SERVER_IS_NOT_VPN`(2)。HTTP 200 以外・Content-Type 不一致も同扱い。

### 1.4 認証 PACK 送信 (Cedar/Protocol.c:ClientUploadAuth, PackLoginWithPassword, PackLoginWithAnonymous, PackAddClientVersion)
`POST /vpnsvc/vpn.cgi` (§3.1) で以下を送る。サーバ側の読み取りは Cedar/Protocol.c:ServerAccept。

**サーバ必須** (無いと失敗):
| 名前 | 型 | 値 |
|---|---|---|
| `method` | STR | `"login"` (無い→`ERR_PROTOCOL_ERROR`) |
| `hubname` | STR | 仮想 HUB 名 (無い→`ERR_PROTOCOL_ERROR`、存在しない→`ERR_HUB_NOT_FOUND`) |
| `username` | STR | ユーザ名 (匿名でも必須。無い→`ERR_PROTOCOL_ERROR`) |
| `authtype` | INT | 0=匿名, 1=パスワード (2=平文パスワード: RADIUS/NT ドメインユーザ用) |
| `secure_password` | DATA 20 | authtype=1 のとき必須。§4 参照。サイズ違い→認証失敗 |

**実質必須** (省略すると INT 0 として扱われ、挙動が変わる):
| 名前 | 型 | 推奨値 | 省略時 |
|---|---|---|---|
| `use_encrypt` | INT | **1** | 0 = データチャネルが TLS を外れて平文になる (§5.5) |
| `max_connection` | INT | **1** | 0 → サーバで 1 に補正 |
| `use_compress` | INT | 0 | 0 |
| `half_connection` | INT | **0** | 0 |
| `qos` | INT(bool) | **0** | 0。1 だとサーバが max_connection を 2 以上に引き上げる |

**省略可能** (本家は送る。ログ/表示用):
| 名前 | 型 | 本家の値 |
|---|---|---|
| `client_str` | STR | `"SoftEther VPN Client Developer Edition"` (src/GlobalConst.h:CEDAR_CLIENT_STR)。**"server" / "bridge" を含めるとブリッジ扱いになる** (大小無視の部分一致) |
| `client_ver` | INT | 502 (= MAJOR*100+MINOR) |
| `client_build` | INT | 5187 (CMakeLists.txt:BUILD_NUMBER 既定。5180 未満は非推奨と警告あり) |
| `hello` | STR | client_str と同じ (サーバはこちらでクライアント名を上書き。無ければ "Unknown") |
| `version` / `build` | INT | client_ver / client_build と同じ |
| `protocol` | INT | 0 (TCP)。サーバは無視して常に TCP 扱い (GetProtocolFromPack) |
| `client_id` | INT | 0 |
| `require_bridge_routing_mode` / `require_monitor_mode` | INT(bool) | 0 / 0 |
| `support_bulk_on_rudp`, `support_hmac_on_bulk_of_rudp`, `support_udp_recovery` | INT(bool) | 1 (R-UDP 用。TCP では無関係。0 でも可) |
| `rudp_bulk_max_version` | INT | 2 (同上) |
| `unique_id` | DATA 20 | マシン固有 SHA-1 (GenerateMachineUniqueHash)。任意の安定した 20 バイトでよい |
| `branded_ctos` | STR | ブランド版のみ。通常送らない |
| `pencore` | DATA | ダミー乱数 0〜999 バイト (§3.3) |
| `use_udp_acceleration` 系 | — | UDP 加速を使わないなら**送らない** |

NODE_INFO (Cedar/Admin.c:OutRpcNodeInfo、全部省略可、サーバのログ表示用):
`ClientProductName`, `ServerProductName`, `ClientOsName`, `ClientOsVer`, `ClientOsProductId`, `ClientHostname`, `ServerHostname`, `ProxyHostname`, `HubName` (以上 STR)、`UniqueId` (DATA 16)、`ClientProductVer`, `ClientProductBuild`, `ServerProductVer`, `ServerProductBuild`, `ClientPort`, `ServerPort2`, `ProxyPort` (INT)、`ClientIpAddress`, `ServerIpAddress`, `ProxyIpAddress` (IP32 形式 §2.4)、`ClientIpAddress6`, `ServerIpAddress6`, `ProxyIpAddress6` (DATA 16)。
- 注意: Ver/Build/Port の INT は本家ではバイトスワップされた値が入る (CreateNodeInfo で Endian32 済みの値を再度 BE で書くため。例 502 → 0xF6010000)。表示用なので正確に合わせる必要はない。

WinVer (Cedar/Admin.c:OutRpcWinVer、省略可): `V_IsWindows`, `V_IsNT`, `V_IsServer`, `V_IsBeta` (INT bool)、`V_VerMajor`, `V_VerMinor`, `V_Build`, `V_ServicePack` (INT)、`V_Title` (STR)。

### 1.5 Welcome 受信 (Cedar/Protocol.c:ClientConnect, ParseWelcomeFromPack, GetSessionKeyFromPack / サーバ側 PackWelcome)
HTTP レスポンスのボディ PACK を以下の順で判定する。
1. `noop` (INT) == 2 の PACK は読み捨てて次のレスポンスを待つ (§3.2)。
2. `error` (INT) != 0 → 失敗。`no_save_password` (INT bool) も付く。
3. `Redirect` (INT) != 0 → クラスタのリダイレクト (§8)。
4. 成功。以下を取得。

| 名前 | 型 | 必須 | 意味 |
|---|---|---|---|
| `session_name` | STR | ○ | セッション名 (例 "SID-USER-1")。無ければプロトコルエラー |
| `connection_name` | STR | ○ | コネクション名 (例 "CID-12") |
| `session_key` | DATA 20 | ○ | 追加コネクション用キー。20 バイトでなければエラー |
| `session_key_32` | INT | | 32bit セッションキー |
| `max_connection` | INT | | 許可 TCP 本数。クライアントは min(これ, 自分の要求, 32) を採用、最低 1 |
| `use_encrypt` | INT | | 1=データも TLS 上、0=TLS を外れて平文 (§5.5) |
| `use_compress` | INT | | 1 ならブロックが zlib 圧縮 (今回は 0 を要求しているので 0 のはず) |
| `half_connection` | INT | | 1 なら片方向コネクション (今回 0) |
| `timeout` | INT | | **ミリ秒**。5000〜60000、既定 30000。KeepAlive 間隔と無通信切断に使う |
| `qos` | INT | | VoIP/QoS |
| `is_azure_session` | INT | | VPN Azure 経由 |
| `vlan_id` | INT | | 割当 VLAN |
| `no_send_signature` | INT bool | | 本家クライアントでは保存のみ (TCP では実質未使用) |
| `enable_udp_recovery` | INT bool | | R-UDP 用 |
| `Msg` | DATA | | UTF-8 (NUL なし) のサーバメッセージ。空でなければユーザに表示 |
| `policy:*` | INT | | ポリシー (`policy:Access`, `policy:MaxConnection`, `policy:TimeOut`, `policy:Ver3` 等。Cedar/Protocol.c:PackGetPolicy) |
| `branded_cfroms` | STR | | ブランド版のみ |
| `use_udp_acceleration` 系 | | | クライアントが要求しなければ来ない |

### 1.6 HTTP → 生ストリームの切り替わり点
- Welcome の HTTP ボディ (Content-Length バイト) を読み終えた**直後のバイトから**、同じ TLS ストリームが §5 のブロック形式になる (クライアント: ClientConnect → StartTunnelingMode、サーバ: ServerAccept で HttpServerSend 直後に StartTunnelingMode)。
- 追加の HTTP やハンドシェイクは無い。サーバは直後から KeepAlive/データを送ってくるので、HTTP 読み取りに使った bufio.Reader をそのまま生ストリーム読み取りに引き継ぐこと (先読み分を捨てない)。

---

## 2. PACK バイナリ形式 (Mayaqua/Pack.c:WritePack/ReadPack/WriteElement/ReadElement/WriteValue/ReadValue, Mayaqua/Memory.c:WriteBufStr/ReadBufStr/WriteBufInt)

### 2.1 構造 (整数はすべて **ビッグエンディアン**)
```
PACK    := u32 num_elements, ELEMENT * num_elements
ELEMENT := u32 name_len_plus_1, name[name_len_plus_1 - 1] (NUL なし),
           u32 type, u32 num_values, VALUE * num_values
```
- **名前の長さの癖**: 書き込み値は `strlen(name)+1` だが、続くバイト列は `strlen(name)` バイトだけ (終端 NUL は書かない)。読み取り側は値から 1 引いた長さを読む。長さ 0 は不正。
- 名前は最大 63 文字 (MAX_ELEMENT_NAME_LEN)。超過分は読み飛ばして切り詰められる。
- 名前の照合は**大小文字無視** (ComparePackName が StrCmpi)。同名 (大小無視) の要素が 2 つあると AddElement が失敗し、**PACK 全体のパースが失敗**する。
- `num_values == 0` の要素も AddElement 失敗 → PACK 全体が失敗。
- 取得時は型も一致必須 (GetElement の type チェック)。例: `use_encrypt` を INT64 で送るとサーバは 0 とみなす。

### 2.2 値型
| type | 名前 | VALUE のエンコード |
|---|---|---|
| 0 | INT | u32 (BE) |
| 1 | DATA | u32 size, bytes[size] |
| 2 | STR | u32 len (= strlen、**+1 なし**), bytes[len] (NUL なし) |
| 3 | UNISTR | u32 size (= UTF-8 バイト長 **+1**), bytes[size] (末尾に NUL 1 バイトを含む) |
| 4 | INT64 | u64 (BE) |
- bool は INT の 0/1 (PackAddBool)。
- STR は ANSI/UTF-8 の生バイト。

### 2.3 配列
同一要素に `num_values` 個の VALUE を並べる (全要素同型)。例: Redirect の `Port`。単一値は num_values=1。

### 2.4 IP アドレス (Mayaqua/Pack.c:PackAddIpEx2) — NODE_INFO 等でのみ使用
名前 `X` に対し 4 要素: `X@ipv6_bool` (INT 0/1)、`X@ipv6_array` (DATA 16: IPv4 は `::ffff:a.b.c.d` 形式)、`X@ipv6_scope_id` (INT)、`X` (INT)。
`X` の INT は「IPv4 の 4 オクテットをリトルエンディアンで解釈した値」を BE で書くため、ワイヤ上はオクテット逆順 (192.168.0.1 → `01 00 A8 C0`)。

### 2.5 サイズ上限
- 要素数 ≤ 262144 (64bit) / 131072 (32bit)、1 要素の値数 ≤ 262144/65536、DATA ≤ 384MiB/96MiB (Mayaqua/Pack.h)。
- **HTTP 経由で実際に効く上限**: サーバが受け付けるクライアント PACK は 65536 バイト以下 (Mayaqua/HTTP.h:HTTP_PACK_MAX_SIZE, HttpServerRecvEx)。クライアント受信側は MAX_PACK_SIZE (512MiB) まで。

---

## 3. HTTP 部分 (Mayaqua/HTTP.c)

### 3.1 クライアント → サーバ (HttpClientSend, PostHttp)
```
POST /vpnsvc/vpn.cgi HTTP/1.1\r\n
Date: <RFC1123 形式>\r\n
Host: <リモート IP 文字列>\r\n
Keep-Alive: timeout=15; max=19\r\n
Connection: Keep-Alive\r\n
Content-Type: application/octet-stream\r\n
Content-Length: <PACK バイト長>\r\n
\r\n
<PACK>
```
サーバ検査 (HttpServerRecvEx): メソッド `POST`、パス `/vpnsvc/vpn.cgi`、バージョン `HTTP/1.1` (いずれも大小無視)、`Content-Type` が `application/octet-stream` と一致 (パラメータ付き不可)、`0 < Content-Length <= 65536`。違反時は 400 系で切断。Date/Host/Keep-Alive は見ていない。

### 3.2 サーバ → クライアント (HttpServerSend, HttpClientRecv)
```
HTTP/1.1 200 OK\r\n
Date: ...\r\n
Keep-Alive: timeout=15; max=19\r\n
Connection: Keep-Alive\r\n
Content-Type: application/octet-stream\r\n
Content-Length: N\r\n
\r\n
<PACK>
```
クライアント側の検査 (本家): ステータス行の 1 トークン目が `HTTP/1.1`、2 トークン目が `200`、Content-Type が `application/octet-stream`、`0 < Content-Length`。エラーも HTTP 200 + `error` 入り PACK で返る点に注意 (HTTP ステータスでは区別されない)。
- **NOOP**: PACK の `noop` (INT) が 2 (NOOP_IGNORE) なら読み捨てて次のレスポンスを待つ。1 セッション 30 回まで (MAX_NOOP_PER_SESSION)。RADIUS 認証が長引くとサーバが送る (Cedar/Radius.c → ServerUploadNoop)。
- ヘッダ行は LF 区切り、末尾 CR は除去される (Mayaqua/Network.c:RecvLine)。1 行 4096 バイトまで。ヘッダ名照合は大小無視。

### 3.3 ダミー乱数パディング (Mayaqua/Network.c:CreateDummyValue)
送信する全 PACK (クライアント・サーバとも) に `pencore` (DATA、長さ `rand() % 1000` バイトの乱数) を追加してからシリアライズする。受信側は無視するので**省略しても動く**。付ける場合はサイズ上限 (65536) に注意。

---

## 4. パスワードハッシュ

- **HashPassword** (Cedar/Account.c:HashPassword):
  `hashed = SHA0( password_bytes || ToUpper(username_bytes) )` — **パスワードが先、ユーザ名が後**、大文字化するのはユーザ名だけ (パスワードは大小区別)。NUL は含めない。大文字化は 1 バイトずつの ASCII `a-z` のみ (Mayaqua/Str.c:StrUpper → ToUpper)。結果 20 バイト。
  - 非 ASCII のバイト表現は本家クライアントの文字コード依存 (Unix 系は UTF-8 と推測)。
- **SecurePassword** (Cedar/Sam.c:SecurePassword):
  `secure_password = SHA0( hashed(20) || random(20, Hello の random) )`。
- サーバ照合 (Cedar/Sam.c:SamAuthUserByPassword): 保存済み HashedKey から同じ計算をして比較。
- **SHA-0 の定義** (Mayaqua/Encrypt.c:MY_SHA0_Transform): SHA-1 とパディング・初期値・ラウンド関数・定数は同一で、メッセージスケジュール `W[t] = W[t-3]^W[t-8]^W[t-14]^W[t-16]` に **1 ビット左ローテートが無い**点だけが異なる。Go 標準には無いので自前実装が必要。
- 匿名認証は secure_password 不要。なおサーバはどの authtype でも**最初に匿名認証を試す** (SamAuthUserByAnonymous) ので、匿名ユーザ名ならパスワード不問で通る。

---

## 5. データチャネル (Cedar/Connection.c)

### 5.1 ブロック形式 (ConnectionSend / ConnectionReceive の Mode 0〜4)
すべて BE の u32。
```
データ:    u32 num_blocks (1..), { u32 size, bytes[size] } * num_blocks
KeepAlive: u32 0xFFFFFFFF, u32 size (0..512), bytes[size]
```
- 各ブロックの中身は **Ethernet フレーム 1 個** (宛先 MAC から。FCS なし、プリアンブルなし)。use_compress=1 の場合のみ zlib 圧縮されたもの。
- 受信時: 先頭 u32 が 0 なら何もしない (次の u32 へ)。0xFFFFFFFF なら KeepAlive。それ以外はブロック数。size 0 のブロックは空として読み飛ばし可。
- 1 回の送信で複数フレームをまとめて 1 つの num_blocks ヘッダにしてよい。

### 5.2 最大フレームサイズ
- MAX_PACKET_SIZE = 1600 (Cedar/Cedar.h)。受信側は size > 3200 で**切断**、1600 < size ≤ 3200 は破棄。送信は 1600 バイト以下にすること (通常の MTU 1500 + Ethernet ヘッダ 14 [+ VLAN 4] で収まる)。

### 5.3 KeepAlive (Cedar/Connection.c:SendKeepAlive, GenNextKeepAliveSpan)
- 形式: magic `0xFFFFFFFF`、size = `rand() % 512` (0〜511)、中身は乱数 (UDP 加速時のみ NAT-T 情報が先頭に入るが今回は無関係)。受信側は size > 512 で切断。
- 送信間隔: `timeout` (Welcome、ms) を a として `max(rand() % (a/2), a/5)`、つまり **a/5 〜 a/2 のランダム**。既定 30000ms なら 6〜15 秒。データブロックを送った場合もタイマーは延長される。サーバも同様に送ってくる。
- 無通信タイムアウト: 最後にデータ/KeepAlive を受信してから `timeout` ms 経過でその TCP を切断 (ConnectionReceive の SOCK_LATER 分岐、Session.c:SessionMain でも同じ値でセッションタイムアウト → `ERR_SESSION_TIMEOUT`)。クライアントもサーバ受信が timeout ms 途絶えたら切断・再接続でよい。

### 5.4 TLS 上の追加暗号化
TCP データチャネルに RC4 等の追加暗号化は**無い**。use_encrypt=1 ならそのまま TLS レコードに載るだけ (Cedar/Connection.c:TcpSockSend/TcpSockRecv → Mayaqua/Network.c:Send/Recv)。RC4/ChaCha20 は UDP 加速専用。

### 5.5 use_encrypt=0 の扱い (重要)
TcpSockSend/TcpSockRecv は `Send(sock, data, size, s->UseEncrypt)` を呼ぶ。UseEncrypt=false だと SSL を使わず**下位の生 TCP ソケットに直接平文で読み書き**する。つまり Welcome 以降は TLS レイヤを素通りして TCP 上に平文ブロックが流れる (TLS セッションは close_notify もせず放置)。Go の crypto/tls でこれに追従するのは面倒なので、**常に use_encrypt=1 を送る**こと。サーバはクライアントの値をそのまま採用する (ServerAccept で `use_encrypt = PackGetInt(p,"use_encrypt")`)。

---

## 6. 追加 TCP コネクション (max_connection > 1、将来用)
(Cedar/Protocol.c:ClientAdditionalConnect, ClientUploadAuth2, PackAdditionalConnect / Session.c:ClientAdditionalConnectChance)
- 新しい TCP+TLS を張り、signature → Hello 受信 (§1.2〜1.3) までは同じ。
- 認証の代わりに `method="additional_connect"` (STR)、`session_key` (DATA 20、Welcome の値)、`client_str`/`client_ver`/`client_build` を POST。
- 応答 PACK: `error` (0 で成功。13/14 ならセッション自体を張り直し)、`direction` (INT: 0=双方向, 1=サーバ→クライアント専用, 2=クライアント→サーバ専用。half_connection 時のみ非 0)。
- 以後そのソケットも §5 のブロック形式。フレームは任意のソケットに分散され、順序保証はない。本家は AdditionalConnectionInterval 秒ごとに 1 本ずつ追加。
- 注意: 本家クライアントは「TCP 本数 < MaxConnection」の状態が一定時間続くとセッションをタイムアウトさせる (Session.c:SessionMain)。1 本で運用するなら max_connection=1 を要求し、qos=0, half_connection=0 にする。

---

## 7. エラーコード (Cedar/Cedar.h)
| 値 | 名前 | 典型的な場面 |
|---|---|---|
| 0 | ERR_NO_ERROR | 成功 |
| 1 | ERR_CONNECT_FAILED | TCP 接続失敗 |
| 2 | ERR_SERVER_IS_NOT_VPN | Hello が取れない / TLS 失敗 |
| 3 | ERR_DISCONNECTED | 途中切断 |
| 4 | ERR_PROTOCOL_ERROR | method/hubname/username 欠落、Welcome 不正 |
| 7 | ERR_AUTHTYPE_NOT_SUPPORTED | ユーザの認証方式と authtype 不一致 |
| 8 | ERR_HUB_NOT_FOUND | HUB が存在しない |
| 9 | ERR_AUTH_FAILED | ユーザ名/パスワード誤り |
| 10 | ERR_HUB_STOPPING | HUB 停止中 |
| 11 | ERR_SESSION_REMOVED | 管理者がセッション削除 |
| 12 | ERR_ACCESS_DENIED | アクセス拒否 (ポリシー等) |
| 13 | ERR_SESSION_TIMEOUT | セッションタイムアウト |
| 14 | ERR_INVALID_PROTOCOL | 不正プロトコル |
| 15 | ERR_TOO_MANY_CONNECTION | コネクション数超過 |
| 16 | ERR_HUB_IS_BUSY | HUB のセッション数上限 |
| 17/18/19 | ERR_PROXY_CONNECT_FAILED / ERR_PROXY_ERROR / ERR_PROXY_AUTH_FAILED | プロキシ関連 |
| 20 | ERR_TOO_MANY_USER_SESSION | 同一ユーザ多重ログイン上限 |
| 23 | ERR_INTERNAL_ERROR | 内部エラー |
| 33 | ERR_NOT_SUPPORTED | サーバが VPN クライアント接続非対応 |
| 63 | ERR_TOO_MANY_USER | サーバのユーザ数上限 |
| 85 | ERR_CERT_NOT_TRUSTED | サーバ証明書が信頼できない (クライアント側判定) |
| 105 | ERR_SERVER_CANT_ACCEPT | HUB の通信無効化など |
| 106 | ERR_SERVER_CERT_EXPIRES | サーバ証明書期限切れ (クライアント側判定) |
| 109 | ERR_IP_ADDRESS_DENIED | 接続元 IP が AC で拒否 |
| 149 | ERR_HOSTNAME_MISMATCH | 証明書ホスト名不一致 (クライアント側判定) |

---

## 8. 実装上の落とし穴
1. **qos / half_connection / max_connection**: 本家クライアントは `qos=1` (DisableQoS=false が既定) を送るため、そのまま真似るとサーバが max_connection≥2 にする。TCP 1 本実装では `qos=0, half_connection=0, max_connection=1` を明示的に送る (Cedar/Protocol.c:ServerAccept の max_connection 補正部)。
2. **use_encrypt=0 は TLS バイパス** (§5.5)。必ず 1。省略も 0 扱いなので必ず送る。
3. **PACK の名前長 +1 と STR/UNISTR の非対称**: 名前は「長さ+1 を書いて NUL は書かない」、STR は「長さそのまま・NUL なし」、UNISTR は「長さ+1・NUL を書く」。
4. **同名要素 (大小無視) や値 0 個の要素を入れると PACK 全体が拒否**される。型違いの要素は「無いもの」扱い。
5. **Welcome 直後の先読みバッファ**: HTTP 解析に使ったリーダの残りバイトは既にデータチャネル (§1.6)。Content-Length 分だけ厳密に読むこと。
6. エラーは HTTP 200 + `error` で返る。`noop==2` の PACK はスキップ (§3.2)。
7. Welcome の `timeout` はミリ秒。KeepAlive をこの 1/2 未満の間隔で送らないとサーバに切断される。
8. HashPassword は「パスワード + 大文字ユーザ名」の順で SHA-0 (SHA-1 ではない)。ユーザ名大文字化は ASCII のみ。
9. `client_str` に "server"/"bridge" (大小無視) を含めない (ブリッジ扱いになる)。
10. **クラスタリダイレクト**: Welcome に `Redirect`(INT)=1 が来たら、`Ip` (IP32 形式 §2.4)、`Port` (INT 配列、現在のポートがあればそれ、無ければ先頭)、`Ticket` (DATA 20)、`Cert` (DATA, X509 DER) を取得 → 空 PACK (pencore のみ) を 1 回 POST → 切断 → 指定先へ再接続し、認証 PACK を `authtype=99` (AUTHTYPE_TICKET) + `ticket` (DATA 20) + `method/hubname/username` で送る。接続先の証明書は `Cert` と一致必須 (Cedar/Protocol.c:ClientConnect の REDIRECTED ラベル周辺)。単一サーバなら来ない。
11. signature の HTTP レスポンスがそのまま Hello なので、signature 送信後に別途応答を待つ処理を入れない。
12. サーバの PACK 受信上限は 65536 バイト。pencore を付けるなら合計がこれを超えないように。
