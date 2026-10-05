// secon-gui はメニューバー (トレイ) 常駐の GUI。デーモンの API を操作するだけ。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/mikuta0407/secon/internal/api"
	"github.com/mikuta0407/secon/internal/engine"
	"github.com/mikuta0407/secon/internal/i18n"
)

// prefLanguage は表示言語の保存キー (値は auto / en / ja)。
const prefLanguage = "language"

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

	status    []engine.Status // 最後に受け取った状態 (デーモンと切れている間も保持する)
	daemonErr error
	seq       uint64 // イベントを受け取るたびに増える (ポーリング結果が古くないかの判定用)
	traySig   string // 最後に描いたトレイの内容 (変化が無ければ作り直さない)
	opening   map[string]bool
	manager   *managerWindow
	editors   map[string]*profileEditor // 接続設定名 → 開いているプロパティウィンドウ (新規は含まない)
	newEditor []*profileEditor
	poll      chan struct{}
}

func main() {
	openManager := flag.Bool("manager", false, "open the connection manager at startup")
	editName := flag.String("edit", "", "open the properties of this profile at startup")
	newProfile := flag.Bool("new", false, "open a new profile window at startup")
	langFlag := flag.String("lang", "", "display language for this run: auto, en or ja (not saved)")
	flag.Parse()

	a := app.NewWithID("io.github.mikuta0407.secon")
	a.Settings().SetTheme(newCompactTheme())
	i18n.Set(i18n.Lang(a.Preferences().StringWithFallback(prefLanguage, string(i18n.Auto))))
	if *langFlag != "" {
		i18n.Set(i18n.Lang(*langFlag))
	}
	desk, ok := a.(desktop.App)
	if !ok {
		log.Fatal("system tray is not supported on this platform")
	}
	g := &gui{app: a, desk: desk, client: api.NewClient(api.ClientSocket()), menu: fyne.NewMenu("secon"), poll: make(chan struct{}, 1), editors: map[string]*profileEditor{}, opening: map[string]bool{}}
	a.Lifecycle().SetOnStarted(func() {
		hideDock()
		// トレイの初期化完了前に設定したアイコンは反映されないことがある (macOS)。
		// 完了を知る API が無いので、起動後に少し待って描き直す
		go func() {
			time.Sleep(2 * time.Second)
			fyne.Do(func() { g.traySig = ""; g.render() })
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
			fyne.Do(func() {
				g.seq++
				g.apply(st)
			})
		})
		if err == nil {
			err = fmt.Errorf("connection to daemon closed")
		}
		log.Printf("daemon: %v", err)
		fyne.Do(func() {
			// 状態は消さずに保持する (再接続後に変化を通知でき、編集中の画面も「削除」扱いにならない)
			g.daemonErr = err
			g.render()
		})
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
		var seq uint64
		fyne.DoAndWait(func() {
			visible = g.daemonErr == nil && ((g.manager != nil && g.manager.visible) || len(g.editors) > 0)
			seq = g.seq
		})
		if !visible {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		st, err := g.client.Status(ctx)
		cancel()
		if err == nil {
			fyne.Do(func() {
				// 取得中にイベントが来ていたら、そちらの方が新しいので捨てる
				if g.seq == seq {
					g.apply(st)
				}
			})
		}
	}
}

func (g *gui) pollNow() {
	select {
	case g.poll <- struct{}{}:
	default:
	}
}

// apply はデーモンから受け取った状態を反映する (main goroutine で呼ぶ)。
func (g *gui) apply(st []engine.Status) {
	g.notifyChanges(st)
	g.status, g.daemonErr = st, nil
	g.render()
	if g.manager != nil {
		g.manager.updateStatus(st)
	}
	for name, e := range g.editors {
		if findStatus(st, name) == nil {
			// 削除された。同名で作り直された接続設定に古いウィンドウを使い回さないよう外す
			delete(g.editors, name)
			e.markDeleted()
			continue
		}
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
			g.app.SendNotification(fyne.NewNotification("secon", i18n.T("notify.connected", s.Name, s.Address)))
		case p == engine.StateConnected && s.State == engine.StateReconnecting:
			g.app.SendNotification(fyne.NewNotification("secon", i18n.T("notify.disconnected", s.Name, s.Error)))
		}
	}
}

// traySignature はトレイに表示する内容の要約。送受信量だけの変化ではメニューを作り直さない
// (作り直すと開いているメニューが閉じたりちらついたりする)。
func (g *gui) traySignature() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%v|", i18n.Current(), g.daemonErr)
	if g.daemonErr == nil {
		for _, s := range g.status {
			fmt.Fprintf(&b, "%s/%s/%s/%s/%s/%v/%s;", s.Name, s.State, s.Mode, s.Address, s.Socks, s.Forwards, s.Error)
		}
	}
	return b.String()
}

// render はトレイのメニューとアイコンを作り直す (内容が変わったときだけ)。
func (g *gui) render() {
	sig := g.traySignature()
	if sig == g.traySig {
		return
	}
	g.traySig = sig
	var items []*fyne.MenuItem
	connected := false

	if g.daemonErr != nil {
		it := fyne.NewMenuItem(i18n.T("tray.noDaemon"), nil)
		it.Disabled = true
		items = append(items, it)
	} else if len(g.status) == 0 {
		it := fyne.NewMenuItem(i18n.T("tray.noProfiles"), nil)
		it.Disabled = true
		items = append(items, it)
	}

	for _, s := range g.status {
		if g.daemonErr != nil {
			break
		}
		if s.State == engine.StateConnected {
			connected = true
		}
		label := fmt.Sprintf("%s %s — %s", stateMark[s.State], s.Name, stateLabel(s.State))
		it := fyne.NewMenuItem(label, nil)
		it.ChildMenu = g.profileMenu(s)
		items = append(items, it)
	}

	quit := fyne.NewMenuItem(i18n.T("tray.quit"), g.app.Quit)
	quit.IsQuit = true
	lang := fyne.NewMenuItem(i18n.T("tray.language"), nil)
	lang.ChildMenu = g.languageMenu()
	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("tray.manager"), g.openManager),
		lang,
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
	info(i18n.T("tray.mode", s.Mode))
	if s.Address != "" {
		info(i18n.T("tray.address", s.Address))
	}
	if s.Socks != "" {
		info(i18n.T("tray.socks", s.Socks))
	}
	for _, f := range s.Forwards {
		info(i18n.T("tray.forward", f))
	}
	if s.Error != "" {
		info(i18n.T("tray.error", s.Error))
	}
	items = append(items, fyne.NewMenuItemSeparator())
	name := s.Name
	if isActive(s.State) {
		items = append(items, fyne.NewMenuItem(i18n.T("btn.disconnect"), func() { g.disconnect(name) }))
	} else {
		items = append(items, fyne.NewMenuItem(i18n.T("btn.connect"), func() { g.connect(name) }))
	}
	items = append(items, fyne.NewMenuItem(i18n.T("tray.properties"), func() { g.openEditor(name) }))
	return fyne.NewMenu(s.Name, items...)
}

// languageMenu は表示言語の切り替えメニュー (自動 / English / 日本語)。
func (g *gui) languageMenu() *fyne.Menu {
	current := i18n.Lang(g.app.Preferences().StringWithFallback(prefLanguage, string(i18n.Auto)))
	var items []*fyne.MenuItem
	for _, l := range []struct {
		lang i18n.Lang
		key  string
	}{{i18n.Auto, "lang.auto"}, {i18n.En, "lang.en"}, {i18n.Ja, "lang.ja"}} {
		it := fyne.NewMenuItem(i18n.T(l.key), func() { g.setLanguage(l.lang) })
		it.Checked = current == l.lang
		items = append(items, it)
	}
	return fyne.NewMenu("", items...)
}

// setLanguage は表示言語を切り替えて保存し、開いているウィンドウを作り直す。
func (g *gui) setLanguage(l i18n.Lang) {
	g.app.Preferences().SetString(prefLanguage, string(l))
	i18n.Set(l)
	g.render()
	if g.manager != nil {
		g.manager.build()
	}
	for _, e := range g.editors {
		e.rebuild()
	}
	for _, e := range g.newEditor {
		e.rebuild()
	}
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
	if g.opening[name] {
		return
	}
	g.opening[name] = true
	// デーモンの応答待ちで UI を止めないよう、取得はバックグラウンドで行う
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, err := g.client.Profiles(ctx)
		fyne.Do(func() {
			delete(g.opening, name)
			if err != nil {
				g.app.SendNotification(fyne.NewNotification("secon", err.Error()))
				return
			}
			if e, ok := g.editors[name]; ok {
				e.show()
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
		})
	}()
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
