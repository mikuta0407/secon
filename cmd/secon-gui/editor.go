package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/engine"
)

// profileEditor は 1 つの接続設定のプロパティウィンドウ (SoftEther クライアントの「接続設定のプロパティ」相当)。
// orig が nil なら新規作成。
type profileEditor struct {
	g    *gui
	w    fyne.Window
	orig *config.Profile

	status     *widget.Label
	details    *detailsView
	connectBtn *widget.Button

	name, server, hub, user, password, cert, proxy *widget.Entry
	socksListen, routes, dnsDomains, forwards      *widget.Entry
	mode                                           *widget.RadioGroup
	autoConnect, insecure, defaultGW, dns          *widget.Check
}

func newProfileEditor(g *gui, orig *config.Profile) *profileEditor {
	e := &profileEditor{g: g, orig: orig}
	title := "新しい接続設定"
	if orig != nil {
		title = "接続設定のプロパティ - " + orig.Name
	}
	e.w = g.app.NewWindow(title)
	e.w.SetCloseIntercept(e.close)

	e.name = widget.NewEntry()
	e.server = widget.NewEntry()
	e.server.SetPlaceHolder("vpn.example.com:443")
	e.hub = widget.NewEntry()
	e.user = widget.NewEntry()
	e.password = widget.NewPasswordEntry()
	e.password.SetPlaceHolder("空なら匿名認証")
	e.cert = widget.NewEntry()
	e.cert.SetPlaceHolder("サーバ証明書の SHA-256 (自己署名のピン留め)")
	e.proxy = widget.NewEntry()
	e.proxy.SetPlaceHolder("http://user:pass@proxy:8080")
	e.mode = widget.NewRadioGroup([]string{config.ModeNIC, config.ModeSocks}, nil)
	e.mode.Horizontal = true
	e.autoConnect = widget.NewCheck("起動時に接続", nil)
	e.insecure = widget.NewCheck("証明書を検証しない", nil)
	e.socksListen = widget.NewEntry()
	e.socksListen.SetPlaceHolder("127.0.0.1:1080")
	e.defaultGW = widget.NewCheck("VPN をデフォルトゲートウェイにする", nil)
	e.dns = widget.NewCheck("VPN 側 DNS を使う", nil)
	e.routes = widget.NewMultiLineEntry()
	e.routes.SetPlaceHolder("10.0.0.0/8 (1 行に 1 つ)")
	e.dnsDomains = widget.NewEntry()
	e.dnsDomains.SetPlaceHolder("corp.example (カンマ区切り)")
	e.forwards = widget.NewMultiLineEntry()
	e.forwards.SetPlaceHolder("127.0.0.1:13389=10.0.0.5:3389 (1 行に 1 つ)")

	form := widget.NewForm(
		widget.NewFormItem("接続設定名", e.name),
		widget.NewFormItem("サーバ", e.server),
		widget.NewFormItem("仮想 HUB", e.hub),
		widget.NewFormItem("ユーザ名", e.user),
		widget.NewFormItem("パスワード", e.password),
		widget.NewFormItem("モード", e.mode),
		widget.NewFormItem("", e.autoConnect),
		widget.NewFormItem("証明書", e.cert),
		widget.NewFormItem("", e.insecure),
		widget.NewFormItem("HTTP Proxy", e.proxy),
		widget.NewFormItem("SOCKS5 待受", e.socksListen),
		widget.NewFormItem("経路", e.routes),
		widget.NewFormItem("", e.defaultGW),
		widget.NewFormItem("", e.dns),
		widget.NewFormItem("DNS ドメイン", e.dnsDomains),
		widget.NewFormItem("ポート転送", e.forwards),
	)

	saveBtn := widget.NewButtonWithIcon("保存", theme.DocumentSaveIcon(), e.save)
	saveBtn.Importance = widget.HighImportance
	cancelBtn := widget.NewButton("キャンセル", e.close)
	buttons := container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn)

	var body fyne.CanvasObject = container.NewVScroll(form)
	var top fyne.CanvasObject
	if orig != nil {
		e.status = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		e.details = newDetailsView()
		e.connectBtn = widget.NewButtonWithIcon("接続", theme.MediaPlayIcon(), e.toggleConnect)
		top = container.NewVBox(container.NewBorder(nil, nil, nil, e.connectBtn, e.status), widget.NewSeparator())
		body = container.NewAppTabs(
			container.NewTabItem("接続設定", body),
			container.NewTabItem("接続情報", container.NewVScroll(e.details.box)),
		)
		e.fill(*orig)
	} else {
		e.fill(config.Profile{Hub: "VPN", Mode: config.ModeSocks})
	}
	e.w.SetContent(container.NewBorder(top, buttons, nil, nil, body))
	e.w.Resize(fyne.NewSize(620, 600))
	return e
}

// origName は編集元の名前 (新規なら "")。
func (e *profileEditor) origName() string {
	if e.orig == nil {
		return ""
	}
	return e.orig.Name
}

func (e *profileEditor) show() {
	activate() // 表示前にアプリを前面化しないと他アプリのウィンドウの後ろに出る (macOS)
	e.w.Show()
	e.w.RequestFocus()
	e.updateStatus(e.g.status)
	e.g.pollNow()
}

func (e *profileEditor) close() {
	e.g.closeEditor(e)
	e.w.Close()
}

func (e *profileEditor) toggleConnect() {
	name := e.origName()
	if st := findStatus(e.g.status, name); st != nil && isActive(st.State) {
		e.g.disconnect(name)
	} else {
		e.g.connect(name)
	}
}

func (e *profileEditor) updateStatus(st []engine.Status) {
	if e.orig == nil {
		return
	}
	x := findStatus(st, e.orig.Name)
	e.details.update(x)
	if x == nil {
		e.status.SetText(e.orig.Name + "  (削除されました)")
		e.connectBtn.Disable()
		return
	}
	text := stateText(x.State)
	if x.Address != "" {
		text += "  " + x.Address
	}
	if x.Error != "" {
		text += "  — " + x.Error
	}
	e.status.SetText(text)
	e.connectBtn.Enable()
	if isActive(x.State) {
		e.connectBtn.SetText("切断")
		e.connectBtn.SetIcon(theme.MediaStopIcon())
	} else {
		e.connectBtn.SetText("接続")
		e.connectBtn.SetIcon(theme.MediaPlayIcon())
	}
}

func (e *profileEditor) fill(p config.Profile) {
	e.name.SetText(p.Name)
	e.server.SetText(p.Server)
	e.hub.SetText(p.Hub)
	e.user.SetText(p.User)
	e.password.SetText(p.Password)
	e.mode.SetSelected(p.Mode)
	e.autoConnect.SetChecked(p.AutoConnect)
	e.cert.SetText(p.CertSHA256)
	e.insecure.SetChecked(p.Insecure)
	e.proxy.SetText(p.Proxy)
	e.socksListen.SetText(p.Socks.Listen)
	e.routes.SetText(strings.Join(p.NIC.Routes, "\n"))
	e.defaultGW.SetChecked(p.NIC.DefaultGateway)
	e.dns.SetChecked(p.NIC.DNS)
	e.dnsDomains.SetText(strings.Join(p.NIC.DNSDomains, ", "))
	var fw []string
	for _, f := range p.Forwards {
		fw = append(fw, f.Listen+"="+f.Target)
	}
	e.forwards.SetText(strings.Join(fw, "\n"))
}

func splitList(s, sep string) []string {
	var out []string
	for _, v := range strings.Split(s, sep) {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (e *profileEditor) collect() (config.Profile, error) {
	p := config.Profile{
		Name:        strings.TrimSpace(e.name.Text),
		Server:      strings.TrimSpace(e.server.Text),
		Hub:         strings.TrimSpace(e.hub.Text),
		User:        strings.TrimSpace(e.user.Text),
		Password:    e.password.Text,
		Mode:        e.mode.Selected,
		AutoConnect: e.autoConnect.Checked,
		CertSHA256:  strings.TrimSpace(e.cert.Text),
		Insecure:    e.insecure.Checked,
		Proxy:       strings.TrimSpace(e.proxy.Text),
		NIC: config.NIC{
			DefaultGateway: e.defaultGW.Checked,
			Routes:         splitList(e.routes.Text, "\n"),
			DNS:            e.dns.Checked,
			DNSDomains:     splitList(e.dnsDomains.Text, ","),
		},
	}
	if p.Mode == config.ModeSocks {
		p.Socks.Listen = strings.TrimSpace(e.socksListen.Text)
	}
	if e.orig != nil {
		// フォームに無い項目は元の値を引き継ぐ
		p.Socks.Username, p.Socks.Password = e.orig.Socks.Username, e.orig.Socks.Password
	}
	for _, line := range splitList(e.forwards.Text, "\n") {
		l, t, ok := strings.Cut(line, "=")
		if !ok {
			return p, fmt.Errorf("ポート転送の書式が不正です: %q (listen=target)", line)
		}
		p.Forwards = append(p.Forwards, config.Forward{Listen: strings.TrimSpace(l), Target: strings.TrimSpace(t)})
	}
	return p, p.Normalize()
}

// save は保存してウィンドウを閉じる。
func (e *profileEditor) save() {
	p, err := e.collect()
	if err != nil {
		dialog.ShowError(err, e.w)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.g.client.PutProfile(ctx, e.origName(), p); err != nil {
		dialog.ShowError(err, e.w)
		return
	}
	if e.g.manager != nil {
		e.g.manager.selected = p.Name
	}
	e.close()
}
