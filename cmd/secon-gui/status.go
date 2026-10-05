package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/mikuta0407/secon/internal/engine"
	"github.com/mikuta0407/secon/internal/i18n"
)

var stateKey = map[engine.State]string{
	engine.StateDisconnected: "state.disconnected",
	engine.StateConnecting:   "state.connecting",
	engine.StateConnected:    "state.connected",
	engine.StateReconnecting: "state.reconnecting",
	engine.StateFailed:       "state.failed",
}

func stateLabel(s engine.State) string { return i18n.T(stateKey[s]) }

var stateMark = map[engine.State]string{
	engine.StateDisconnected: "○",
	engine.StateConnecting:   "◐",
	engine.StateConnected:    "●",
	engine.StateReconnecting: "◐",
	engine.StateFailed:       "✕",
}

func stateText(s engine.State) string { return stateMark[s] + " " + stateLabel(s) }

// isActive は接続を試みている (切断操作の対象になる) 状態か。
func isActive(s engine.State) bool {
	return s == engine.StateConnecting || s == engine.StateConnected || s == engine.StateReconnecting
}

func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// uptime は接続済みなら接続からの経過時間を返す。
func uptime(s engine.Status) string {
	if s.State != engine.StateConnected {
		return ""
	}
	return formatDuration(time.Since(s.Since))
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// detailRows は状態の詳細を (項目, 値) の組で返す。
func detailRows(s engine.Status) [][2]string {
	var rows [][2]string
	add := func(key, v string) {
		if v != "" {
			rows = append(rows, [2]string{i18n.T(key), v})
		}
	}
	add("detail.state", stateText(s.State))
	add("detail.server", s.Server)
	add("detail.hubUser", s.Hub+" / "+s.User)
	add("detail.mode", s.Mode)
	add("detail.error", s.Error)
	add("detail.address", s.Address)
	add("detail.gateway", s.Gateway)
	add("detail.dns", strings.Join(s.DNS, ", "))
	add("detail.interface", s.Interface)
	add("detail.socks", s.Socks)
	for _, f := range s.Forwards {
		add("detail.forward", f)
	}
	add("detail.session", s.Session)
	add("detail.serverInfo", s.ServerInfo)
	if s.State == engine.StateConnected {
		add("detail.uptime", uptime(s))
		add("detail.received", i18n.T("detail.traffic", formatBytes(s.BytesIn), s.PacketsIn))
		add("detail.sent", i18n.T("detail.traffic", formatBytes(s.BytesOut), s.PacketsOut))
	} else if s.State != engine.StateDisconnected {
		add("detail.elapsed", formatDuration(time.Since(s.Since)))
	}
	return rows
}

// detailsView は状態の詳細を「項目: 値」の 2 段組で表示する部品。
type detailsView struct {
	box *fyne.Container
}

func newDetailsView() *detailsView {
	return &detailsView{box: container.NewGridWithColumns(2)}
}

func (d *detailsView) update(s *engine.Status) {
	if s == nil {
		d.box.Objects = []fyne.CanvasObject{widget.NewLabel(i18n.T("detail.selectProfile"))}
		d.box.Refresh()
		return
	}
	rows := detailRows(*s)
	half := (len(rows) + 1) / 2
	col := func(rs [][2]string) fyne.CanvasObject {
		c := container.New(layout.NewFormLayout())
		for _, r := range rs {
			k := widget.NewLabelWithStyle(r[0], fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
			v := widget.NewLabel(r[1])
			v.Wrapping = fyne.TextWrapWord
			v.Selectable = true
			c.Add(k)
			c.Add(v)
		}
		return container.NewVBox(c)
	}
	d.box.Objects = []fyne.CanvasObject{col(rows[:half]), col(rows[half:])}
	d.box.Refresh()
}

func findStatus(st []engine.Status, name string) *engine.Status {
	for i := range st {
		if st[i].Name == name {
			return &st[i]
		}
	}
	return nil
}
