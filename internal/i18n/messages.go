package i18n

// messages はメッセージ key → {英語, 日本語}。日本語が空なら英語を使う。
var messages = map[string][2]string{
	// 設定ファイルの検証
	"cfg.profileNameRequired":  {"profile #%d: enter a profile name", "%d 番目の接続設定: 接続設定名を入力してください"},
	"cfg.duplicateName":        {"duplicate profile name %q", "接続設定名 %q が重複しています"},
	"cfg.profileError":         {"profile %q: %s", "接続設定 %q: %s"},
	"cfg.nameRequired":         {"enter a profile name", "接続設定名を入力してください"},
	"cfg.serverRequired":       {"enter a server", "サーバを入力してください"},
	"cfg.hubRequired":          {"enter a virtual hub", "仮想 HUB を入力してください"},
	"cfg.userRequired":         {"enter a user name", "ユーザ名を入力してください"},
	"cfg.unknownMode":          {"unknown mode %q (nic or socks)", "モード %q は不明です (nic または socks)"},
	"cfg.invalidProxy":         {"invalid HTTP proxy %q (http://host:port)", "HTTP Proxy の形式が不正です: %q (http://host:port)"},
	"cfg.invalidRoute":         {"invalid route %q (e.g. 10.0.0.0/8)", "経路の形式が不正です: %q (例 10.0.0.0/8)"},
	"cfg.invalidForwardListen": {"invalid port forward listen address %q (e.g. 127.0.0.1:13389)", "ポート転送の待受アドレスが不正です: %q (例 127.0.0.1:13389)"},
	"cfg.invalidForwardTarget": {"invalid port forward target %q (e.g. 10.0.0.5:3389)", "ポート転送の転送先が不正です: %q (例 10.0.0.5:3389)"},
	"cfg.staticNeedsAddress":   {"enter an IP address to use a static address", "固定アドレスを使うには IP アドレスを入力してください"},
	"cfg.invalidAddress":       {"invalid IP address %q (e.g. 10.0.0.50/24)", "IP アドレスの形式が不正です: %q (例 10.0.0.50/24)"},
	"cfg.networkAddress":       {"%q is a network address (e.g. 10.0.0.50/24)", "%q はネットワークアドレスです (例 10.0.0.50/24)"},
	"cfg.invalidGateway":       {"invalid gateway %q", "ゲートウェイの形式が不正です: %q"},
	"cfg.gatewayOutside":       {"gateway %s is outside the subnet of %s", "ゲートウェイ %s が %s のサブネットの外にあります"},
	"cfg.invalidDNS":           {"invalid DNS server %q", "DNS サーバの形式が不正です: %q"},

	// CLI
	"cli.usage": {`usage: secon <command> [args]

commands:
  status [profile]       list profiles, or show details of one
  connect <profile>      connect (waits until connected)
  disconnect <profile>   disconnect
  reload                 make the daemon re-read the config file
  service install|uninstall [--user]
                         register / remove the daemon (launchd / systemd)
  daemon [flags]         run the daemon (normally started by launchd / systemd)
  version                show the version
  debug dump|socks       connect without the daemon (for troubleshooting)

Set SECON_LANG=en or SECON_LANG=ja to choose the language.
`, `usage: secon <command> [args]

commands:
  status [接続設定名]     一覧、または 1 件の詳細
  connect <接続設定名>    接続する (完了まで待つ)
  disconnect <接続設定名> 切断する
  reload                 デーモンに設定ファイルを再読み込みさせる
  service install|uninstall [--user]
                         デーモンを登録・解除する (launchd / systemd)
  daemon [flags]         デーモンとして起動する (通常は launchd / systemd から)
  version                バージョンを表示する
  debug dump|socks       デーモンを使わずに直接接続する (調査用)

表示言語は SECON_LANG=en / SECON_LANG=ja で切り替えられます。
`},
	"cli.noProfiles":     {"no profiles", "接続設定がありません"},
	"cli.noSuchProfile":  {"no such profile: %s", "接続設定がありません: %s"},
	"cli.connected":      {"connected: %s (%s)", "接続しました: %s (%s)"},
	"cli.waitTimeout":    {"timed out waiting for the connection (the daemon keeps trying)", "接続の完了待ちがタイムアウトしました (デーモンは再試行を続けます)"},
	"cli.connectFailed":  {"%s: %s (the daemon keeps retrying; run 'secon disconnect %s' to stop)", "%s: %s (デーモンが再試行を続けます。止めるには 'secon disconnect %s')"},
	"cli.createdConfig":  {"created %s (edit it, then run 'secon reload')", "%s を作成しました (編集したら 'secon reload' を実行してください)"},
	"cli.installed":      {"installed %s", "登録しました: %s"},
	"cli.removed":        {"removed %s", "削除しました: %s"},
	"cli.daemonNotFound": {"the daemon is not running (%s); start it with 'secon service install'", "デーモンが動いていません (%s)。'secon service install' で登録してください"},
	"cli.permission":     {"permission denied on %s: add your user to group %q (or set api.group in the config)", "%s にアクセスできません: ユーザをグループ %q に追加してください (または設定の api.group)"},
	"cli.sampleConfig": {`# secon config file. After editing, run 'secon reload'.
#
# [[profile]]
# name = "office"
# server = "vpn.example.com:443"
# hub = "VPN"
# user = "alice"
# password = "secret"
# mode = "nic"                # "nic" (virtual NIC, needs root) or "socks"
# auto_connect = true
# cert_sha256 = ""            # SHA-256 of the server certificate (pin a self-signed cert)
# proxy = ""                  # http://user:pass@proxy:8080
# static = { address = "10.0.0.50/24", gateway = "10.0.0.1", dns = ["10.0.0.1"] }  # omit to use DHCP
# nic = { routes = ["10.0.0.0/8"], default_gateway = false, dns = true }
# socks = { listen = "127.0.0.1:1080" }
#
# [[profile.forward]]
# listen = "127.0.0.1:13389"
# target = "10.0.0.5:3389"
`, `# secon 設定ファイル。編集後は 'secon reload' で反映する。
#
# [[profile]]
# name = "office"
# server = "vpn.example.com:443"
# hub = "VPN"
# user = "alice"
# password = "secret"
# mode = "nic"                # "nic" (仮想 NIC, root 必要) または "socks"
# auto_connect = true
# cert_sha256 = ""            # サーバ証明書の SHA-256 (自己署名証明書のピン留め)
# proxy = ""                  # http://user:pass@proxy:8080
# static = { address = "10.0.0.50/24", gateway = "10.0.0.1", dns = ["10.0.0.1"] }  # 省略すると DHCP
# nic = { routes = ["10.0.0.0/8"], default_gateway = false, dns = true }
# socks = { listen = "127.0.0.1:1080" }
#
# [[profile.forward]]
# listen = "127.0.0.1:13389"
# target = "10.0.0.5:3389"
`},

	// 接続状態
	"state.disconnected": {"Disconnected", "切断"},
	"state.connecting":   {"Connecting…", "接続中…"},
	"state.connected":    {"Connected", "接続済み"},
	"state.reconnecting": {"Reconnecting…", "再接続中…"},
	"state.failed":       {"Error", "エラー"},

	// 接続情報
	"detail.state":         {"State", "状態"},
	"detail.server":        {"Server", "接続先"},
	"detail.hubUser":       {"Hub / user", "仮想 HUB / ユーザ"},
	"detail.mode":          {"Mode", "モード"},
	"detail.error":         {"Error", "エラー"},
	"detail.address":       {"IP address", "IP アドレス"},
	"detail.gateway":       {"Gateway", "ゲートウェイ"},
	"detail.dns":           {"DNS", ""},
	"detail.interface":     {"Virtual NIC", "仮想 NIC"},
	"detail.socks":         {"SOCKS5", ""},
	"detail.forward":       {"Port forward", "ポート転送"},
	"detail.session":       {"Session", "セッション"},
	"detail.serverInfo":    {"Server build", "サーバ"},
	"detail.uptime":        {"Uptime", "接続時間"},
	"detail.received":      {"Received", "受信"},
	"detail.sent":          {"Sent", "送信"},
	"detail.elapsed":       {"Elapsed", "経過"},
	"detail.traffic":       {"%s (%d packets)", "%s (%d パケット)"},
	"detail.selectProfile": {"Select a profile", "接続設定を選択してください"},

	// 接続マネージャ
	"mgr.title":         {"secon Connection Manager", "secon 接続マネージャ"},
	"mgr.infoTitle":     {"Connection info: %s", "%s の接続情報"},
	"mgr.deleteTitle":   {"Delete", "削除"},
	"mgr.deleteConfirm": {"Delete profile %q?", "接続設定 %q を削除しますか?"},
	"col.name":          {"Name", "接続設定名"},
	"col.state":         {"State", "状態"},
	"col.mode":          {"Mode", "モード"},
	"col.server":        {"Server", "接続先サーバ"},
	"col.hub":           {"Hub", "仮想 HUB"},
	"col.address":       {"IP address", "IP アドレス"},
	"col.received":      {"Received", "受信"},
	"col.sent":          {"Sent", "送信"},
	"col.uptime":        {"Uptime", "接続時間"},

	// ボタン
	"btn.connect":    {"Connect", "接続"},
	"btn.disconnect": {"Disconnect", "切断"},
	"btn.new":        {"New", "新規"},
	"btn.properties": {"Properties", "プロパティ"},
	"btn.delete":     {"Delete", "削除"},
	"btn.save":       {"Save", "保存"},
	"btn.cancel":     {"Cancel", "キャンセル"},

	// プロパティ
	"ed.newTitle":         {"New Profile", "新しい接続設定"},
	"ed.propsTitle":       {"Profile Properties - %s", "接続設定のプロパティ - %s"},
	"ed.deleted":          {"%s (deleted)", "%s (削除されました)"},
	"ed.invalidForward":   {"invalid port forward %q (listen=target)", "ポート転送の書式が不正です: %q (listen=target)"},
	"tab.settings":        {"Settings", "接続設定"},
	"tab.info":            {"Connection info", "接続情報"},
	"field.name":          {"Name", "接続設定名"},
	"field.server":        {"Server", "サーバ"},
	"field.hub":           {"Virtual hub", "仮想 HUB"},
	"field.user":          {"User", "ユーザ名"},
	"field.password":      {"Password", "パスワード"},
	"field.mode":          {"Mode", "モード"},
	"field.cert":          {"Certificate", "証明書"},
	"field.proxy":         {"HTTP proxy", "HTTP Proxy"},
	"field.addressing":    {"IP address", "IP アドレス"},
	"field.staticAddress": {"Address", "アドレス"},
	"field.staticGateway": {"Gateway", "ゲートウェイ"},
	"field.staticDNS":     {"DNS servers", "DNS サーバ"},
	"field.socksListen":   {"SOCKS5 listen", "SOCKS5 待受"},
	"field.routes":        {"Routes", "経路"},
	"field.dnsDomains":    {"DNS domains", "DNS ドメイン"},
	"field.forwards":      {"Port forwards", "ポート転送"},
	"ph.password":         {"Empty = anonymous", "空なら匿名認証"},
	"ph.cert":             {"Server certificate SHA-256 (pin a self-signed cert)", "サーバ証明書の SHA-256 (自己署名のピン留め)"},
	"ph.staticDNS":        {"10.0.0.1 (comma separated)", "10.0.0.1 (カンマ区切り)"},
	"ph.routes":           {"10.0.0.0/8 (one per line)", "10.0.0.0/8 (1 行に 1 つ)"},
	"ph.dnsDomains":       {"corp.example (comma separated)", "corp.example (カンマ区切り)"},
	"ph.forwards":         {"127.0.0.1:13389=10.0.0.5:3389 (one per line)", "127.0.0.1:13389=10.0.0.5:3389 (1 行に 1 つ)"},
	"chk.autoConnect":     {"Connect at startup", "起動時に接続"},
	"chk.insecure":        {"Do not verify the certificate", "証明書を検証しない"},
	"chk.defaultGW":       {"Use the VPN as the default gateway", "VPN をデフォルトゲートウェイにする"},
	"chk.dns":             {"Use the VPN's DNS", "VPN 側 DNS を使う"},
	"addr.dhcp":           {"DHCP", ""},
	"addr.static":         {"Static", "固定"},

	// トレイ
	"tray.noDaemon":   {"Cannot reach the daemon", "デーモンに接続できません"},
	"tray.noProfiles": {"No profiles", "接続設定がありません"},
	"tray.manager":    {"Connection Manager…", "接続マネージャ…"},
	"tray.quit":       {"Quit", "終了"},
	"tray.properties": {"Properties…", "プロパティ…"},
	"tray.mode":       {"Mode: %s", "モード: %s"},
	"tray.address":    {"Address: %s", "アドレス: %s"},
	"tray.socks":      {"SOCKS5: %s", ""},
	"tray.forward":    {"Forward: %s", "転送: %s"},
	"tray.error":      {"Error: %s", "エラー: %s"},
	"tray.language":   {"Language / 言語", ""},
	"lang.auto":       {"Automatic", "自動"},
	"lang.en":         {"English", ""},
	"lang.ja":         {"日本語", ""},

	// 通知
	"notify.connected":    {"Connected to %s (%s)", "%s に接続しました (%s)"},
	"notify.disconnected": {"%s was disconnected: %s", "%s が切断されました: %s"},
}
