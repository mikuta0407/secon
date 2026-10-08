package main

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"time"

	"fyne.io/fyne/v2"

	"github.com/mikuta0407/secon/internal/api"
	"github.com/mikuta0407/secon/internal/i18n"
	"github.com/mikuta0407/secon/internal/service"
)

// daemonKind は GUI から起動・停止するデーモンの種類。
type daemonKind int

const (
	daemonNone   daemonKind = iota // 操作しない (未登録・SECON_SOCKET 指定など)
	daemonUser                     // ユーザデーモン: 起動も停止もそのまま行える
	daemonSystem                   // システムデーモン: 停止は API、起動は管理者認証
)

// daemonSocket は GUI が繋ぐソケットを決める。どのデーモンも動いていなければ登録済みのものを選ぶ
// (デーモンが止まっている間に起動した GUI が、後から起動したデーモンに繋がれるように)。
func daemonSocket() string {
	s := api.ClientSocket()
	if os.Getenv("SECON_SOCKET") != "" || reachable(s) {
		return s
	}
	switch {
	case service.Installed(false):
		return api.SystemSocket
	case service.Installed(true):
		return api.UserSocket()
	}
	return s
}

// reachable はソケットの先でデーモンが動いているかを返す (権限が無くて繋げない場合も動いているとみなす)。
func reachable(socket string) bool {
	c, err := net.DialTimeout("unix", socket, time.Second)
	if err == nil {
		c.Close()
		return true
	}
	return errors.Is(err, fs.ErrPermission)
}

// daemonKind は GUI が繋いでいるデーモンの種類を、ソケットと登録の有無から決める。
func (g *gui) daemonKind() daemonKind {
	if os.Getenv("SECON_SOCKET") != "" {
		return daemonNone
	}
	switch g.client.Socket {
	case api.SystemSocket:
		if service.Installed(false) {
			return daemonSystem
		}
	case api.UserSocket():
		if service.Installed(true) {
			return daemonUser
		}
	}
	return daemonNone
}

// startDaemon はデーモンを起動する。システムデーモンなら管理者認証のダイアログが出る。
func (g *gui) startDaemon() {
	kind := g.daemonKind()
	g.runDaemonOp(false, func() error {
		if kind == daemonSystem {
			return service.StartSystem(i18n.T("auth.startDaemon"))
		}
		return service.Start(true)
	})
}

// stopDaemon はデーモンを止める (次回ログイン・起動時にはまた動く)。quit なら止まった後に GUI も終了する。
func (g *gui) stopDaemon(quit bool) {
	g.runDaemonOp(quit, g.shutdownDaemon)
}

// shutdownDaemon はデーモンに停止を頼み、実際に止まる (ソケットに繋がらなくなる) まで最大 15 秒待つ。
func (g *gui) shutdownDaemon() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := g.client.Shutdown(ctx)
	cancel()
	if err != nil {
		return err
	}
	for i := 0; i < 75; i++ {
		if !reachable(g.client.Socket) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return i18n.Errorf("err.daemonStillRunning")
}

// runDaemonOp は op を別 goroutine で実行する (launchctl や認証ダイアログで数秒以上かかるため)。
// 実行中はトレイの操作項目を無効にする。認証のキャンセルは失敗として通知しない。
func (g *gui) runDaemonOp(quit bool, op func() error) {
	g.daemonOp = true
	g.render()
	go func() {
		err := op()
		fyne.Do(func() {
			g.daemonOp = false
			switch {
			case errors.Is(err, service.ErrCanceled):
			case err != nil:
				g.app.SendNotification(fyne.NewNotification("secon", err.Error()))
			case quit:
				g.app.Quit()
				return
			}
			g.render()
		})
	}()
}
