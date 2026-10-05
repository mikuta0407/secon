// secon-gui はメニューバー (トレイ) 常駐の GUI。デーモンの API を操作するだけ。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/mikuta0407/secon/internal/api"
	"github.com/mikuta0407/secon/internal/engine"
)

type gui struct {
	app    fyne.App
	desk   desktop.App
	client *api.Client

	// トレイのメニューは同じオブジェクトを使い回して Items だけ差し替える。
	// Fyne はトレイ初期化完了時に最初に渡された Menu で作り直すため、
	// 毎回新しい Menu を渡すと起動直後の更新が古いメニューで上書きされる (macOS で確認)。
	menu *fyne.Menu

	status    []engine.Status
	daemonErr error
	settings  *settingsWindow
}

func main() {
	openSettings := flag.Bool("settings", false, "起動時に設定画面を開く")
	flag.Parse()

	a := app.NewWithID("io.github.mikuta0407.secon")
	desk, ok := a.(desktop.App)
	if !ok {
		log.Fatal("system tray is not supported on this platform")
	}
	g := &gui{app: a, desk: desk, client: api.NewClient(api.ClientSocket()), menu: fyne.NewMenu("secon")}
	a.Lifecycle().SetOnStarted(func() {
		hideDock()
		// トレイの初期化完了前に設定したアイコンは反映されないことがある (macOS)。
		// 完了を知る API が無いので、起動後に少し待って描き直す
		go func() {
			time.Sleep(2 * time.Second)
			fyne.Do(g.render)
		}()
	})
	g.render()
	go g.watch()
	if *openSettings {
		g.openSettings()
	}
	a.Run()
}

// watch はデーモンのイベントを購読し、切れたら再接続する。
func (g *gui) watch() {
	for {
		log.Printf("connecting to daemon (%s)", g.client.Socket)
		err := g.client.Events(context.Background(), func(st []engine.Status) {
			fyne.Do(func() {
				g.notifyChanges(st)
				g.status, g.daemonErr = st, nil
				g.render()
			})
		})
		if err == nil {
			err = fmt.Errorf("connection to daemon closed")
		}
		log.Printf("daemon: %v", err)
		fyne.Do(func() {
			g.status, g.daemonErr = nil, err
			g.render()
		})
		time.Sleep(3 * time.Second)
	}
}

// notifyChanges は接続・切断を通知センターに出す。
func (g *gui) notifyChanges(next []engine.Status) {
	prev := map[string]engine.State{}
	for _, s := range g.status {
		prev[s.Name] = s.State
	}
	for _, s := range next {
		p, ok := prev[s.Name]
		if !ok || p == s.State {
			continue
		}
		switch {
		case s.State == engine.StateConnected:
			g.app.SendNotification(fyne.NewNotification("secon", fmt.Sprintf("%s に接続しました (%s)", s.Name, s.Address)))
		case p == engine.StateConnected && s.State == engine.StateReconnecting:
			g.app.SendNotification(fyne.NewNotification("secon", fmt.Sprintf("%s が切断されました: %s", s.Name, s.Error)))
		}
	}
}

var stateLabel = map[engine.State]string{
	engine.StateDisconnected: "切断",
	engine.StateConnecting:   "接続中…",
	engine.StateConnected:    "接続済み",
	engine.StateReconnecting: "再接続中…",
	engine.StateFailed:       "エラー",
}

var stateMark = map[engine.State]string{
	engine.StateDisconnected: "○",
	engine.StateConnecting:   "◐",
	engine.StateConnected:    "●",
	engine.StateReconnecting: "◐",
	engine.StateFailed:       "✕",
}

// render はトレイのメニューとアイコンを作り直す。
func (g *gui) render() {
	var items []*fyne.MenuItem
	connected := false

	if g.daemonErr != nil {
		it := fyne.NewMenuItem("デーモンに接続できません", nil)
		it.Disabled = true
		items = append(items, it)
	} else if len(g.status) == 0 {
		it := fyne.NewMenuItem("プロファイルがありません", nil)
		it.Disabled = true
		items = append(items, it)
	}

	for _, s := range g.status {
		if s.State == engine.StateConnected {
			connected = true
		}
		label := fmt.Sprintf("%s %s — %s", stateMark[s.State], s.Name, stateLabel[s.State])
		it := fyne.NewMenuItem(label, nil)
		it.ChildMenu = g.profileMenu(s)
		items = append(items, it)
	}

	quit := fyne.NewMenuItem("終了", g.app.Quit)
	quit.IsQuit = true
	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("設定…", g.openSettings),
		quit,
	)
	g.menu.Items = items
	g.desk.SetSystemTrayMenu(g.menu)
	if connected {
		g.desk.SetSystemTrayIcon(iconConnected)
	} else {
		g.desk.SetSystemTrayIcon(iconDisconnected)
	}
	if g.settings != nil {
		g.settings.updateStatus(g.status)
	}
}

func (g *gui) profileMenu(s engine.Status) *fyne.Menu {
	var items []*fyne.MenuItem
	info := func(text string) {
		it := fyne.NewMenuItem(text, nil)
		it.Disabled = true
		items = append(items, it)
	}
	info("モード: " + s.Mode)
	if s.Address != "" {
		info("アドレス: " + s.Address)
	}
	if s.Socks != "" {
		info("SOCKS5: " + s.Socks)
	}
	for _, f := range s.Forwards {
		info("転送: " + f)
	}
	if s.Error != "" {
		info("エラー: " + s.Error)
	}
	items = append(items, fyne.NewMenuItemSeparator())
	name := s.Name
	if s.State == engine.StateDisconnected || s.State == engine.StateFailed {
		items = append(items, fyne.NewMenuItem("接続", func() { g.call(func(ctx context.Context) error { return g.client.Connect(ctx, name) }) }))
	} else {
		items = append(items, fyne.NewMenuItem("切断", func() { g.call(func(ctx context.Context) error { return g.client.Disconnect(ctx, name) }) }))
	}
	return fyne.NewMenu(s.Name, items...)
}

// call は API をバックグラウンドで呼び、失敗したら通知する。
func (g *gui) call(f func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := f(ctx); err != nil {
			fyne.Do(func() {
				g.app.SendNotification(fyne.NewNotification("secon", err.Error()))
			})
		}
	}()
}

func (g *gui) openSettings() {
	if g.settings == nil {
		g.settings = newSettingsWindow(g)
	}
	g.settings.show()
}
