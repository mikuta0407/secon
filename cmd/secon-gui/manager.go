package main

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mikuta0407/secon/internal/engine"
	"github.com/mikuta0407/secon/internal/i18n"
)

// managerWindow は SoftEther VPN Client マネージャ風の接続一覧 (メイン画面)。
type managerWindow struct {
	g       *gui
	w       fyne.Window
	visible bool

	status   []engine.Status
	selected string
	selRow   int    // 表で選択表示になっている行 (-1 は無し)。selected と食い違わないよう同期する
	pending  string // 保存直後で、まだ状態一覧に現れていない選択対象

	table   *widget.Table
	details *detailsView
	title   *widget.Label

	connectBtn, disconnectBtn, editBtn, deleteBtn *widget.Button
}

var managerColumns = []struct {
	key   string
	width float32
	value func(s engine.Status) string
}{
	{"col.name", 120, func(s engine.Status) string { return s.Name }},
	{"col.state", 110, func(s engine.Status) string { return stateText(s.State) }},
	{"col.mode", 60, func(s engine.Status) string { return s.Mode }},
	{"col.server", 150, func(s engine.Status) string { return s.Server }},
	{"col.hub", 75, func(s engine.Status) string { return s.Hub }},
	{"col.address", 120, func(s engine.Status) string { return dash(s.Address) }},
	{"col.received", 80, func(s engine.Status) string { return bytesOrDash(s, s.BytesIn) }},
	{"col.sent", 80, func(s engine.Status) string { return bytesOrDash(s, s.BytesOut) }},
	{"col.uptime", 80, func(s engine.Status) string { return dash(uptime(s)) }},
}

func bytesOrDash(s engine.Status, n uint64) string {
	if s.State != engine.StateConnected {
		return "-"
	}
	return formatBytes(n)
}

func newManagerWindow(g *gui) *managerWindow {
	m := &managerWindow{g: g, selRow: -1}
	m.w = g.app.NewWindow("")
	m.w.SetCloseIntercept(func() { m.visible = false; m.w.Hide() })
	m.build()
	m.w.Resize(fyne.NewSize(900, 560))
	return m
}

// build はウィンドウの中身を現在の言語で作る (言語切り替え時にも呼ぶ)。
func (m *managerWindow) build() {
	m.w.SetTitle(i18n.T("mgr.title"))
	m.table = widget.NewTableWithHeaders(
		func() (int, int) { return len(m.status), len(managerColumns) },
		func() fyne.CanvasObject { return newTableCell(m) },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			c := o.(*tableCell)
			s := m.status[id.Row]
			c.row = id.Row
			c.SetText(managerColumns[id.Col].value(s))
			c.TextStyle.Bold = s.Name == m.selected
			c.Refresh()
		},
	)
	m.table.ShowHeaderColumn = false
	m.table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("header", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	m.table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		if id.Col >= 0 {
			o.(*widget.Label).SetText(i18n.T(managerColumns[id.Col].key))
		}
	}
	for i, c := range managerColumns {
		m.table.SetColumnWidth(i, c.width)
	}
	m.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(m.status) {
			m.selRow = id.Row
			m.selected = m.status[id.Row].Name
			m.pending = ""
			m.refresh()
		}
	}
	m.selRow = -1

	m.connectBtn = widget.NewButtonWithIcon(i18n.T("btn.connect"), theme.MediaPlayIcon(), func() { m.g.connect(m.selected) })
	m.disconnectBtn = widget.NewButtonWithIcon(i18n.T("btn.disconnect"), theme.MediaStopIcon(), func() { m.g.disconnect(m.selected) })
	newBtn := widget.NewButtonWithIcon(i18n.T("btn.new"), theme.ContentAddIcon(), func() { m.g.openEditor("") })
	m.editBtn = widget.NewButtonWithIcon(i18n.T("btn.properties"), theme.DocumentCreateIcon(), func() { m.g.openEditor(m.selected) })
	m.deleteBtn = widget.NewButtonWithIcon(i18n.T("btn.delete"), theme.DeleteIcon(), m.remove)
	m.connectBtn.Importance = widget.HighImportance
	toolbar := container.NewHBox(m.connectBtn, m.disconnectBtn, widget.NewSeparator(), newBtn, m.editBtn, m.deleteBtn, layout.NewSpacer())

	m.details = newDetailsView()
	m.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	detailPane := container.NewBorder(m.title, nil, nil, nil, container.NewVScroll(m.details.box))

	split := container.NewVSplit(m.table, detailPane)
	split.Offset = 0.45
	m.w.SetContent(container.NewBorder(container.NewVBox(toolbar, widget.NewSeparator()), nil, nil, nil, split))
	m.syncSelection()
	m.refresh()
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
	if findStatus(st, m.selected) != nil {
		if m.selected == m.pending {
			m.pending = ""
		}
	} else if m.selected == "" || m.selected != m.pending {
		m.selected = ""
		if len(st) > 0 {
			m.selected = st[0].Name
		}
	}
	m.syncSelection()
	m.refresh()
}

// selectName は name を選択する (保存直後で一覧にまだ無くても、現れたときに選択される)。
func (m *managerWindow) selectName(name string) {
	m.selected, m.pending = name, name
	m.updateStatus(m.status)
}

// syncSelection は表の選択表示を m.selected に合わせる。
func (m *managerWindow) syncSelection() {
	row := -1
	for i, s := range m.status {
		if s.Name == m.selected {
			row = i
		}
	}
	if row == m.selRow {
		return
	}
	m.selRow = row
	if row < 0 {
		m.table.UnselectAll()
		return
	}
	m.table.Select(widget.TableCellID{Row: row, Col: 0})
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
	m.title.SetText(i18n.T("mgr.infoTitle", s.Name))
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

// tableCell は表のセル。クリックで行を選択し、ダブルクリックでプロパティ、右クリックで操作メニューを出す。
// (Table 既定の選択処理の代わりにセル自身がタップを受ける)
type tableCell struct {
	widget.Label
	m   *managerWindow
	row int
}

func newTableCell(m *managerWindow) *tableCell {
	c := &tableCell{m: m}
	c.Text = "placeholder-text"
	c.ExtendBaseWidget(c)
	return c
}

func (c *tableCell) Tapped(*fyne.PointEvent) { c.m.selectRow(c.row) }

func (c *tableCell) DoubleTapped(*fyne.PointEvent) {
	c.m.selectRow(c.row)
	c.m.g.openEditor(c.m.selected)
}

func (c *tableCell) TappedSecondary(e *fyne.PointEvent) {
	c.m.selectRow(c.row)
	c.m.showContextMenu(e.AbsolutePosition)
}

func (m *managerWindow) selectRow(row int) {
	if row >= 0 && row < len(m.status) {
		m.table.Select(widget.TableCellID{Row: row, Col: 0})
	}
}

// showContextMenu は選択中の接続設定の操作メニュー (接続/切断・プロパティ・削除) を出す。
func (m *managerWindow) showContextMenu(pos fyne.Position) {
	s := findStatus(m.status, m.selected)
	if s == nil {
		return
	}
	name := s.Name
	connect := fyne.NewMenuItem(i18n.T("btn.connect"), func() { m.g.connect(name) })
	disconnect := fyne.NewMenuItem(i18n.T("btn.disconnect"), func() { m.g.disconnect(name) })
	if isActive(s.State) {
		connect.Disabled = true
	} else {
		disconnect.Disabled = true
	}
	menu := fyne.NewMenu("",
		connect,
		disconnect,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("tray.properties"), func() { m.g.openEditor(name) }),
		fyne.NewMenuItem(i18n.T("btn.delete")+"…", m.remove),
	)
	widget.ShowPopUpMenuAtPosition(menu, m.w.Canvas(), pos)
}

func (m *managerWindow) remove() {
	name := m.selected
	if name == "" {
		return
	}
	dialog.ShowConfirm(i18n.T("mgr.deleteTitle"), i18n.T("mgr.deleteConfirm", name), func(ok bool) {
		if !ok {
			return
		}
		// デーモンは接続の切断を待ってから応答するので、UI を止めないようバックグラウンドで送る
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if err := m.g.client.DeleteProfile(ctx, name); err != nil {
				fyne.Do(func() { dialog.ShowError(err, m.w) })
			}
		}()
	}, m.w)
}
