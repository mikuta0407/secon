package main

import (
	"context"
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
	"github.com/mikuta0407/secon/internal/i18n"
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
	staticAddr, staticGW, staticDNS                *widget.Entry
	socksListen, routes, dnsDomains, forwards      *widget.Entry
	mode, addressing                               *widget.RadioGroup
	autoConnect, insecure, defaultGW, dns          *widget.Check

	form               *fyne.Container
	staticRows         []formRow
	socksRows, nicRows []formRow
	dnsDomainRow       formRow
}

// formRow はフォームの 1 行 (ラベルと入力欄)。
type formRow struct{ label, input fyne.CanvasObject }

func (r formRow) setVisible(v bool) {
	for _, o := range []fyne.CanvasObject{r.label, r.input} {
		if v {
			o.Show()
		} else {
			o.Hide()
		}
	}
}

func newProfileEditor(g *gui, orig *config.Profile) *profileEditor {
	e := &profileEditor{g: g, orig: orig}
	e.w = g.app.NewWindow("")
	e.w.SetCloseIntercept(e.close)
	e.build()
	if orig != nil {
		e.fill(*orig)
	} else {
		e.fill(config.Profile{Hub: "VPN", Mode: config.ModeNIC})
	}
	e.w.Resize(fyne.NewSize(620, 620))
	return e
}

// build はウィンドウの中身を現在の言語で作る。
func (e *profileEditor) build() {
	if e.orig == nil {
		e.w.SetTitle(i18n.T("ed.newTitle"))
	} else {
		e.w.SetTitle(i18n.T("ed.propsTitle", e.orig.Name))
	}

	entry := func(placeholder string) *widget.Entry {
		w := widget.NewEntry()
		w.SetPlaceHolder(placeholder)
		return w
	}
	multi := func(placeholder string) *widget.Entry {
		w := widget.NewMultiLineEntry()
		w.SetPlaceHolder(placeholder)
		return w
	}
	e.name = entry("")
	e.server = entry("vpn.example.com:443")
	e.hub = entry("")
	e.user = entry("")
	e.password = widget.NewPasswordEntry()
	e.password.SetPlaceHolder(i18n.T("ph.password"))
	e.cert = entry(i18n.T("ph.cert"))
	e.proxy = entry("http://user:pass@proxy:8080")
	e.mode = widget.NewRadioGroup([]string{config.ModeNIC, config.ModeSocks}, nil)
	e.mode.Horizontal = true
	e.addressing = widget.NewRadioGroup([]string{i18n.T("addr.dhcp"), i18n.T("addr.static")}, nil)
	e.addressing.Horizontal = true
	e.staticAddr = entry("10.0.0.50/24")
	e.staticGW = entry("10.0.0.1")
	e.staticDNS = entry(i18n.T("ph.staticDNS"))
	e.autoConnect = widget.NewCheck(i18n.T("chk.autoConnect"), nil)
	e.insecure = widget.NewCheck(i18n.T("chk.insecure"), nil)
	e.socksListen = entry("127.0.0.1:1080")
	e.defaultGW = widget.NewCheck(i18n.T("chk.defaultGW"), nil)
	e.dns = widget.NewCheck(i18n.T("chk.dns"), nil)
	e.routes = multi(i18n.T("ph.routes"))
	e.dnsDomains = entry(i18n.T("ph.dnsDomains"))
	e.forwards = multi(i18n.T("ph.forwards"))

	// モードなどによって使う項目だけを表示する。widget.Form は行を隠せないので
	// FormLayout で組む (ラベルと入力欄の両方が非表示の行は詰めて表示される)
	e.form = container.New(layout.NewFormLayout())
	row := func(key string, w fyne.CanvasObject) formRow {
		text := ""
		if key != "" {
			text = i18n.T(key)
		}
		l := widget.NewLabelWithStyle(text, fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
		e.form.Add(l)
		e.form.Add(w)
		return formRow{l, w}
	}
	row("field.name", e.name)
	row("field.server", e.server)
	row("field.hub", e.hub)
	row("field.user", e.user)
	row("field.password", e.password)
	row("field.mode", e.mode)
	row("", e.autoConnect)
	row("field.cert", e.cert)
	row("", e.insecure)
	row("field.proxy", e.proxy)
	row("field.addressing", e.addressing)
	e.staticRows = []formRow{
		row("field.staticAddress", e.staticAddr),
		row("field.staticGateway", e.staticGW),
		row("field.staticDNS", e.staticDNS),
	}
	e.socksRows = []formRow{row("field.socksListen", e.socksListen)}
	e.nicRows = []formRow{row("field.routes", e.routes), row("", e.defaultGW), row("", e.dns)}
	e.dnsDomainRow = row("field.dnsDomains", e.dnsDomains)
	row("field.forwards", e.forwards)
	e.mode.OnChanged = func(string) { e.updateVisibility() }
	e.addressing.OnChanged = func(string) { e.updateVisibility() }
	e.dns.OnChanged = func(bool) { e.updateVisibility() }

	saveBtn := widget.NewButtonWithIcon(i18n.T("btn.save"), theme.DocumentSaveIcon(), e.save)
	saveBtn.Importance = widget.HighImportance
	cancelBtn := widget.NewButton(i18n.T("btn.cancel"), e.close)
	buttons := container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn)

	var body fyne.CanvasObject = container.NewVScroll(e.form)
	var top fyne.CanvasObject
	if e.orig != nil {
		e.status = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		e.details = newDetailsView()
		e.connectBtn = widget.NewButtonWithIcon(i18n.T("btn.connect"), theme.MediaPlayIcon(), e.toggleConnect)
		top = container.NewVBox(container.NewBorder(nil, nil, nil, e.connectBtn, e.status), widget.NewSeparator())
		body = container.NewAppTabs(
			container.NewTabItem(i18n.T("tab.settings"), body),
			container.NewTabItem(i18n.T("tab.info"), container.NewVScroll(e.details.box)),
		)
	}
	e.w.SetContent(container.NewBorder(top, buttons, nil, nil, body))
}

func (e *profileEditor) entries() []*widget.Entry {
	return []*widget.Entry{e.name, e.server, e.hub, e.user, e.password, e.cert, e.proxy,
		e.staticAddr, e.staticGW, e.staticDNS, e.socksListen, e.routes, e.dnsDomains, e.forwards}
}

func (e *profileEditor) checks() []*widget.Check {
	return []*widget.Check{e.autoConnect, e.insecure, e.defaultGW, e.dns}
}

// rebuild は入力中の値を保ったまま現在の言語で作り直す。
func (e *profileEditor) rebuild() {
	var texts []string
	for _, w := range e.entries() {
		texts = append(texts, w.Text)
	}
	var checks []bool
	for _, c := range e.checks() {
		checks = append(checks, c.Checked)
	}
	mode, static := e.mode.Selected, e.isStatic()

	e.build()
	for i, w := range e.entries() {
		w.SetText(texts[i])
	}
	for i, c := range e.checks() {
		c.SetChecked(checks[i])
	}
	e.mode.SetSelected(mode)
	e.setStatic(static)
	e.updateVisibility()
	e.updateStatus(e.g.status)
}

// isStatic は「固定」を選んでいるか。表示文字列は言語で変わるので選択肢の位置で判定する
// (言語切り替え直後の rebuild でも正しく判定できるように)。
func (e *profileEditor) isStatic() bool {
	return e.addressing.Selected != "" && e.addressing.Selected == e.addressing.Options[1]
}

func (e *profileEditor) setStatic(v bool) {
	i := 0
	if v {
		i = 1
	}
	e.addressing.SetSelected(e.addressing.Options[i])
}

// updateVisibility は選択中のモード・アドレス取得方法で使う項目だけを表示する。
func (e *profileEditor) updateVisibility() {
	nic := e.mode.Selected == config.ModeNIC
	for _, r := range e.staticRows {
		r.setVisible(e.isStatic())
	}
	for _, r := range e.socksRows {
		r.setVisible(!nic)
	}
	for _, r := range e.nicRows {
		r.setVisible(nic)
	}
	e.dnsDomainRow.setVisible(nic && e.dns.Checked)
	e.form.Refresh()
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
		e.status.SetText(i18n.T("ed.deleted", e.orig.Name))
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
		e.connectBtn.SetText(i18n.T("btn.disconnect"))
		e.connectBtn.SetIcon(theme.MediaStopIcon())
	} else {
		e.connectBtn.SetText(i18n.T("btn.connect"))
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
	e.setStatic(p.Static.Enabled())
	e.staticAddr.SetText(p.Static.Address)
	e.staticGW.SetText(p.Static.Gateway)
	e.staticDNS.SetText(strings.Join(p.Static.DNS, ", "))
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
	e.updateVisibility()
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
	if e.isStatic() {
		p.Static = config.Static{
			Address: strings.TrimSpace(e.staticAddr.Text),
			Gateway: strings.TrimSpace(e.staticGW.Text),
			DNS:     splitList(e.staticDNS.Text, ","),
		}
	}
	// 表示していない (別モード用の) 項目は保存しない
	if p.Mode == config.ModeSocks {
		p.Socks.Listen = strings.TrimSpace(e.socksListen.Text)
		p.NIC = config.NIC{}
	} else if !p.NIC.DNS {
		p.NIC.DNSDomains = nil
	}
	if e.orig != nil {
		// フォームに無い項目は元の値を引き継ぐ
		p.Socks.Username, p.Socks.Password = e.orig.Socks.Username, e.orig.Socks.Password
	}
	for _, line := range splitList(e.forwards.Text, "\n") {
		l, t, ok := strings.Cut(line, "=")
		if !ok {
			return p, i18n.Errorf("ed.invalidForward", line)
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
