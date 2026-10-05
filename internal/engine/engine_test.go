package engine

import (
	"testing"
	"time"

	"github.com/mikuta0407/secon/internal/config"
)

// 接続ループ中のプロファイルを設定変更で止めてもデッドロックしないこと。
func TestApplyStopsRunningProfileWithoutDeadlock(t *testing.T) {
	m := NewManager()
	m.Logf = t.Logf
	p := config.Profile{Name: "a", Server: "127.0.0.1:1", Hub: "VPN", User: "u", Mode: config.ModeSocks, AutoConnect: true}
	cfg := &config.Config{Profiles: []config.Profile{p}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Profiles[0].Socks.Listen = "127.0.0.1:0"
	m.Apply(cfg)
	time.Sleep(200 * time.Millisecond) // 接続失敗→再接続待ちに入る

	done := make(chan struct{})
	go func() {
		m.Apply(&config.Config{}) // プロファイル削除
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Apply deadlocked while stopping a running profile")
	}
	if st := m.Status(); len(st) != 0 {
		t.Errorf("status = %+v", st)
	}
}
