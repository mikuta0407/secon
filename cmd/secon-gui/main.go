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

const (
	ctxSecond    = time.Second
	pollInterval = 2 * time.Second // ウィンドウ表示中に送受信量などを更新する間隔
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
	manager   *managerWindow
	editors   map[string]*profileEditor // 接続設定名 → 開いているプロパティウィンドウ (新規は含まない)
	newEditor []*profileEditor
	poll      chan struct{}
}

func main() {
	openManager := flag.Bool("manager", false, "起動時に接続マネージャを開く")
	editName := flag.String("edit", "", "起動時にこの接続設定のプロパティを開く (確認用)")
	newProfile := flag.Bool("new", false, "起動時に新しい接続設定のウィンドウを開く (確認用)")
	flag.Parse()

	a := app.NewWithID("io.github.mikuta0407.secon")
	a.Settings().SetTheme(newCompactTheme())
	desk, ok := a.(desktop.App)
	if !ok {
		log.Fatal("system tray is not supported on this platform")
	}
	g := &gui{app: a, desk: desk, client: api.NewClient(api.ClientSocket()), menu: fyne.NewMenu("secon"), poll: make(chan struct{}, 1), editors: map[string]*profileEditor{}}
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
	go g.pollLoop()
	if *openManager {
		g.openManager()
	}
	if *editName != "" {
		g.openEditor(*editName)
	}
	if *newProfile {
		g.openEditor("")
	}
	a.Run()
}

// watch はデーモンのイベントを購読し、切れたら再接続する。
func (g *gui) watch() {
	for {
		log.Printf("connecting to daemon (%s)", g.client.Socket)
		err := g.client.Events(context.Background(), func(st []engine.Status) {
			fyne.Do(func() { g.apply(st, nil) })
		})
		if err == nil {
			err = fmt.Errorf("connection to daemon closed")
		}
		log.Printf("daemon: %v", err)
		fyne.Do(func() { g.apply(nil, err) })
		time.Sleep(3 * time.Second)
	}
}

// pollLoop はウィンドウ表示中だけ状態を取り直す (送受信量・接続時間はイベントが来ないため)。
func (g *gui) pollLoop() {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-g.poll:
		}
		visible := false
		fyne.DoAndWait(func() {
			visible = (g.manager != nil && g.manager.visible) || len(g.editors) > 0
		})
		if !visible {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		st, err := g.client.Status(ctx)
		cancel()
		if err == nil {
			fyne.Do(func() { g.apply(st, nil) })
		}
	}
}

func (g *gui) pollNow() {
	select {
	case g.poll <- struct{}{}:
	default:
	}
}

func (g *gui) apply(st []engine.Status, err error) {
	if err == nil {
		g.notifyChanges(st)
	}
	g.status, g.daemonErr = st, err
	g.render()
	if g.manager != nil {
		g.manager.updateStatus(st)
	}
	for _, e := range g.editors {
		e.updateStatus(st)
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
		fyne.NewMenuItem("接続マネージャ…", g.openManager),
		quit,
	)
	g.menu.Items = items
	g.desk.SetSystemTrayMenu(g.menu)
	if connected {
		g.desk.SetSystemTrayIcon(iconConnected)
	} else {
		g.desk.SetSystemTrayIcon(iconDisconnected)
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
	if isActive(s.State) {
		items = append(items, fyne.NewMenuItem("切断", func() { g.disconnect(name) }))
	} else {
		items = append(items, fyne.NewMenuItem("接続", func() { g.connect(name) }))
	}
	items = append(items, fyne.NewMenuItem("プロパティ…", func() { g.openEditor(name) }))
	return fyne.NewMenu(s.Name, items...)
}

func (g *gui) connect(name string) {
	if name != "" {
		g.call(func(ctx context.Context) error { return g.client.Connect(ctx, name) })
	}
}

func (g *gui) disconnect(name string) {
	if name != "" {
		g.call(func(ctx context.Context) error { return g.client.Disconnect(ctx, name) })
	}
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

// openEditor は接続設定のプロパティウィンドウを開く。name が "" なら新規作成。
// 同じ接続設定のウィンドウが既に開いていれば前面に出す。
func (g *gui) openEditor(name string) {
	if name == "" {
		e := newProfileEditor(g, nil)
		g.newEditor = append(g.newEditor, e)
		e.show()
		return
	}
	if e, ok := g.editors[name]; ok {
		e.show()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ps, err := g.client.Profiles(ctx)
	if err != nil {
		g.app.SendNotification(fyne.NewNotification("secon", err.Error()))
		return
	}
	for _, p := range ps {
		if p.Name == name {
			e := newProfileEditor(g, &p)
			g.editors[name] = e
			e.show()
			return
		}
	}
}

func (g *gui) closeEditor(e *profileEditor) {
	delete(g.editors, e.origName())
	for i, x := range g.newEditor {
		if x == e {
			g.newEditor = append(g.newEditor[:i], g.newEditor[i+1:]...)
			break
		}
	}
}

func (g *gui) openManager() {
	if g.manager == nil {
		g.manager = newManagerWindow(g)
		g.manager.updateStatus(g.status)
	}
	g.manager.show()
}
