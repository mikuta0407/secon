package main

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mikuta0407/secon/internal/engine"
)

// managerWindow は SoftEther VPN Client マネージャ風の接続一覧。
type managerWindow struct {
	g       *gui
	w       fyne.Window
	visible bool

	status   []engine.Status
	selected string

	table   *widget.Table
	details *detailsView
	title   *widget.Label

	connectBtn, disconnectBtn, editBtn, deleteBtn *widget.Button
}

var managerColumns = []struct {
	title string
	width float32
	value func(s engine.Status) string
}{
	{"接続設定名", 120, func(s engine.Status) string { return s.Name }},
	{"状態", 100, func(s engine.Status) string { return stateText(s.State) }},
	{"モード", 60, func(s engine.Status) string { return s.Mode }},
	{"接続先サーバ", 150, func(s engine.Status) string { return s.Server }},
	{"仮想 HUB", 75, func(s engine.Status) string { return s.Hub }},
	{"IP アドレス", 120, func(s engine.Status) string { return dash(s.Address) }},
	{"受信", 80, func(s engine.Status) string { return bytesOrDash(s, s.BytesIn) }},
	{"送信", 80, func(s engine.Status) string { return bytesOrDash(s, s.BytesOut) }},
	{"接続時間", 80, func(s engine.Status) string { return dash(uptime(s)) }},
}

func bytesOrDash(s engine.Status, n uint64) string {
	if s.State != engine.StateConnected {
		return "-"
	}
	return formatBytes(n)
}

func newManagerWindow(g *gui) *managerWindow {
	m := &managerWindow{g: g}
	m.w = g.app.NewWindow("secon 接続マネージャ")
	m.w.SetCloseIntercept(func() { m.visible = false; m.w.Hide() })

	m.table = widget.NewTableWithHeaders(
		func() (int, int) { return len(m.status), len(managerColumns) },
		func() fyne.CanvasObject { return widget.NewLabel("placeholder-text") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			s := m.status[id.Row]
			l.SetText(managerColumns[id.Col].value(s))
			l.TextStyle.Bold = s.Name == m.selected
			l.Refresh()
		},
	)
	m.table.ShowHeaderColumn = false
	m.table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("header", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	m.table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		if id.Col >= 0 {
			o.(*widget.Label).SetText(managerColumns[id.Col].title)
		}
	}
	for i, c := range managerColumns {
		m.table.SetColumnWidth(i, c.width)
	}
	m.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(m.status) {
			m.selected = m.status[id.Row].Name
			m.refresh()
		}
	}

	m.connectBtn = widget.NewButtonWithIcon("接続", theme.MediaPlayIcon(), func() { m.g.connect(m.selected) })
	m.disconnectBtn = widget.NewButtonWithIcon("切断", theme.MediaStopIcon(), func() { m.g.disconnect(m.selected) })
	newBtn := widget.NewButtonWithIcon("新規", theme.ContentAddIcon(), func() { m.g.openEditor("") })
	m.editBtn = widget.NewButtonWithIcon("プロパティ", theme.DocumentCreateIcon(), func() { m.g.openEditor(m.selected) })
	m.deleteBtn = widget.NewButtonWithIcon("削除", theme.DeleteIcon(), m.remove)
	m.connectBtn.Importance = widget.HighImportance
	toolbar := container.NewHBox(m.connectBtn, m.disconnectBtn, widget.NewSeparator(), newBtn, m.editBtn, m.deleteBtn, layout.NewSpacer())

	m.details = newDetailsView()
	m.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	detailPane := container.NewBorder(m.title, nil, nil, nil, container.NewVScroll(m.details.box))

	split := container.NewVSplit(m.table, detailPane)
	split.Offset = 0.45
	m.w.SetContent(container.NewBorder(container.NewVBox(toolbar, widget.NewSeparator()), nil, nil, nil, split))
	m.w.Resize(fyne.NewSize(900, 560))
	return m
}

func (m *managerWindow) show() {
	m.visible = true
	activate()
	m.w.Show()
	m.w.RequestFocus()
	m.g.pollNow()
}

func (m *managerWindow) updateStatus(st []engine.Status) {
	m.status = st
	if m.selected == "" || findStatus(st, m.selected) == nil {
		m.selected = ""
		if len(st) > 0 {
			m.selected = st[0].Name
		}
	}
	m.refresh()
}

func (m *managerWindow) refresh() {
	m.table.Refresh()
	s := findStatus(m.status, m.selected)
	m.details.update(s)
	if s == nil {
		m.title.SetText("")
		for _, b := range []*widget.Button{m.connectBtn, m.disconnectBtn, m.editBtn, m.deleteBtn} {
			b.Disable()
		}
		return
	}
	m.title.SetText(fmt.Sprintf("%s の接続情報", s.Name))
	m.editBtn.Enable()
	m.deleteBtn.Enable()
	if isActive(s.State) {
		m.connectBtn.Disable()
		m.disconnectBtn.Enable()
	} else {
		m.connectBtn.Enable()
		m.disconnectBtn.Disable()
	}
}

func (m *managerWindow) remove() {
	name := m.selected
	if name == "" {
		return
	}
	dialog.ShowConfirm("削除", fmt.Sprintf("接続設定 %q を削除しますか?", name), func(ok bool) {
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*ctxSecond)
		defer cancel()
		if err := m.g.client.DeleteProfile(ctx, name); err != nil {
			dialog.ShowError(err, m.w)
		}
	}, m.w)
}
