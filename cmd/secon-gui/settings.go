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
	"fyne.io/fyne/v2/widget"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/engine"
)

// settingsWindow はプロファイルの一覧と編集フォーム。
type settingsWindow struct {
	g        *gui
	w        fyne.Window
	profiles []config.Profile
	selected int // -1 は新規
	list     *widget.List
	status   *widget.Label

	name, server, hub, user, password, cert, proxy *widget.Entry
	socksListen, routes, dnsDomains, forwards      *widget.Entry
	mode                                           *widget.RadioGroup
	autoConnect, insecure, defaultGW, dns          *widget.Check
}

func newSettingsWindow(g *gui) *settingsWindow {
	s := &settingsWindow{g: g, selected: -1}
	s.w = g.app.NewWindow("secon 設定")
	s.w.SetCloseIntercept(s.w.Hide)

	s.name = widget.NewEntry()
	s.server = widget.NewEntry()
	s.server.SetPlaceHolder("vpn.example.com:443")
	s.hub = widget.NewEntry()
	s.user = widget.NewEntry()
	s.password = widget.NewPasswordEntry()
	s.password.SetPlaceHolder("空なら匿名認証")
	s.cert = widget.NewEntry()
	s.cert.SetPlaceHolder("サーバ証明書の SHA-256 (自己署名のピン留め)")
	s.proxy = widget.NewEntry()
	s.proxy.SetPlaceHolder("http://user:pass@proxy:8080")
	s.mode = widget.NewRadioGroup([]string{config.ModeNIC, config.ModeSocks}, nil)
	s.mode.Horizontal = true
	s.autoConnect = widget.NewCheck("起動時に接続", nil)
	s.insecure = widget.NewCheck("証明書を検証しない", nil)
	s.socksListen = widget.NewEntry()
	s.socksListen.SetPlaceHolder("127.0.0.1:1080")
	s.defaultGW = widget.NewCheck("VPN をデフォルトゲートウェイにする", nil)
	s.dns = widget.NewCheck("VPN 側 DNS を使う", nil)
	s.routes = widget.NewMultiLineEntry()
	s.routes.SetPlaceHolder("10.0.0.0/8 (1 行に 1 つ)")
	s.dnsDomains = widget.NewEntry()
	s.dnsDomains.SetPlaceHolder("corp.example (カンマ区切り)")
	s.forwards = widget.NewMultiLineEntry()
	s.forwards.SetPlaceHolder("127.0.0.1:13389=10.0.0.5:3389 (1 行に 1 つ)")

	form := widget.NewForm(
		widget.NewFormItem("名前", s.name),
		widget.NewFormItem("サーバ", s.server),
		widget.NewFormItem("仮想 HUB", s.hub),
		widget.NewFormItem("ユーザ名", s.user),
		widget.NewFormItem("パスワード", s.password),
		widget.NewFormItem("モード", s.mode),
		widget.NewFormItem("", s.autoConnect),
		widget.NewFormItem("証明書", s.cert),
		widget.NewFormItem("", s.insecure),
		widget.NewFormItem("HTTP Proxy", s.proxy),
		widget.NewFormItem("SOCKS5 待受", s.socksListen),
		widget.NewFormItem("経路", s.routes),
		widget.NewFormItem("", s.defaultGW),
		widget.NewFormItem("", s.dns),
		widget.NewFormItem("DNS ドメイン", s.dnsDomains),
		widget.NewFormItem("ポート転送", s.forwards),
	)

	s.list = widget.NewList(
		func() int { return len(s.profiles) },
		func() fyne.CanvasObject { return widget.NewLabel("profile-name-placeholder") },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(s.profiles[i].Name) },
	)
	s.list.OnSelected = func(i widget.ListItemID) { s.selected = i; s.fill(s.profiles[i]) }

	s.status = widget.NewLabel("")
	newBtn := widget.NewButton("新規", func() {
		s.list.UnselectAll()
		s.selected = -1
		s.fill(config.Profile{Hub: "VPN", Mode: config.ModeSocks})
	})
	saveBtn := widget.NewButton("保存", s.save)
	saveBtn.Importance = widget.HighImportance
	delBtn := widget.NewButton("削除", s.remove)

	left := container.NewBorder(nil, newBtn, nil, nil, s.list)
	s.status.Wrapping = fyne.TextWrapWord
	buttons := container.NewHBox(layout.NewSpacer(), delBtn, saveBtn)
	right := container.NewBorder(nil, container.NewVBox(s.status, buttons), nil, nil, container.NewVScroll(form))
	split := container.NewHSplit(left, right)
	split.Offset = 0.25
	s.w.SetContent(split)
	s.w.Resize(fyne.NewSize(760, 640))
	return s
}

func (s *settingsWindow) show() {
	s.reload("")
	activate() // 表示前にアプリを前面化しないと他アプリのウィンドウの後ろに出る (macOS)
	s.w.Show()
	s.w.RequestFocus()
}

// reload はデーモンからプロファイルを読み直し、name を選択する。
func (s *settingsWindow) reload(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ps, err := s.g.client.Profiles(ctx)
	if err != nil {
		dialog.ShowError(err, s.w)
		return
	}
	s.profiles = ps
	s.list.Refresh()
	s.selected = -1
	for i, p := range ps {
		if p.Name == name || (name == "" && i == 0) {
			s.list.Select(i)
			return
		}
	}
	s.fill(config.Profile{Hub: "VPN", Mode: config.ModeSocks})
}

func (s *settingsWindow) updateStatus(st []engine.Status) {
	if s.selected < 0 || s.selected >= len(s.profiles) {
		s.status.SetText("")
		return
	}
	for _, x := range st {
		if x.Name == s.profiles[s.selected].Name {
			text := stateMark[x.State] + " " + stateLabel[x.State]
			if x.Address != "" {
				text += "  " + x.Address
			}
			if x.Error != "" {
				text += "  — " + x.Error
			}
			s.status.SetText(text)
		}
	}
}

func (s *settingsWindow) fill(p config.Profile) {
	s.name.SetText(p.Name)
	s.server.SetText(p.Server)
	s.hub.SetText(p.Hub)
	s.user.SetText(p.User)
	s.password.SetText(p.Password)
	s.mode.SetSelected(p.Mode)
	s.autoConnect.SetChecked(p.AutoConnect)
	s.cert.SetText(p.CertSHA256)
	s.insecure.SetChecked(p.Insecure)
	s.proxy.SetText(p.Proxy)
	s.socksListen.SetText(p.Socks.Listen)
	s.routes.SetText(strings.Join(p.NIC.Routes, "\n"))
	s.defaultGW.SetChecked(p.NIC.DefaultGateway)
	s.dns.SetChecked(p.NIC.DNS)
	s.dnsDomains.SetText(strings.Join(p.NIC.DNSDomains, ", "))
	var fw []string
	for _, f := range p.Forwards {
		fw = append(fw, f.Listen+"="+f.Target)
	}
	s.forwards.SetText(strings.Join(fw, "\n"))
	s.updateStatus(s.g.status)
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

func (s *settingsWindow) collect() (config.Profile, error) {
	p := config.Profile{
		Name:        strings.TrimSpace(s.name.Text),
		Server:      strings.TrimSpace(s.server.Text),
		Hub:         strings.TrimSpace(s.hub.Text),
		User:        strings.TrimSpace(s.user.Text),
		Password:    s.password.Text,
		Mode:        s.mode.Selected,
		AutoConnect: s.autoConnect.Checked,
		CertSHA256:  strings.TrimSpace(s.cert.Text),
		Insecure:    s.insecure.Checked,
		Proxy:       strings.TrimSpace(s.proxy.Text),
		NIC: config.NIC{
			DefaultGateway: s.defaultGW.Checked,
			Routes:         splitList(s.routes.Text, "\n"),
			DNS:            s.dns.Checked,
			DNSDomains:     splitList(s.dnsDomains.Text, ","),
		},
	}
	if p.Mode == config.ModeSocks {
		p.Socks.Listen = strings.TrimSpace(s.socksListen.Text)
	}
	if i := s.selected; i >= 0 && i < len(s.profiles) {
		// フォームに無い項目は元の値を引き継ぐ
		p.Socks.Username, p.Socks.Password = s.profiles[i].Socks.Username, s.profiles[i].Socks.Password
	}
	for _, line := range splitList(s.forwards.Text, "\n") {
		l, t, ok := strings.Cut(line, "=")
		if !ok {
			return p, fmt.Errorf("ポート転送の書式が不正です: %q (listen=target)", line)
		}
		p.Forwards = append(p.Forwards, config.Forward{Listen: strings.TrimSpace(l), Target: strings.TrimSpace(t)})
	}
	return p, p.Normalize()
}

func (s *settingsWindow) save() {
	p, err := s.collect()
	if err != nil {
		dialog.ShowError(err, s.w)
		return
	}
	old := ""
	if s.selected >= 0 && s.selected < len(s.profiles) {
		old = s.profiles[s.selected].Name
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.g.client.PutProfile(ctx, old, p); err != nil {
		dialog.ShowError(err, s.w)
		return
	}
	s.reload(p.Name)
}

func (s *settingsWindow) remove() {
	if s.selected < 0 || s.selected >= len(s.profiles) {
		return
	}
	name := s.profiles[s.selected].Name
	dialog.ShowConfirm("削除", fmt.Sprintf("プロファイル %q を削除しますか?", name), func(ok bool) {
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.g.client.DeleteProfile(ctx, name); err != nil {
			dialog.ShowError(err, s.w)
			return
		}
		s.reload("")
	}, s.w)
}
