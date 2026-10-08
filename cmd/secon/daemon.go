package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"sync"
	"syscall"

	"github.com/mikuta0407/secon/internal/api"
	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/engine"
	"github.com/mikuta0407/secon/internal/i18n"
	"github.com/mikuta0407/secon/internal/nic"
	"github.com/mikuta0407/secon/internal/service"
)

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "設定ファイル")
	socket := fs.String("socket", "", "制御ソケット (既定: 設定ファイルの api.socket か "+api.DefaultSocket()+")")
	fs.Parse(args)

	// 設定ファイルに誤りがあっても起動する (起動失敗→再起動を繰り返さないように)。
	// その場合は接続設定なしで動き、修正後の 'secon reload' で反映する
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Printf("config: %v (starting without profiles; fix it and run 'secon reload')", err)
		cfg = &config.Config{}
	}
	if *socket == "" {
		*socket = cfg.API.Socket
	}
	if *socket == "" {
		*socket = api.DefaultSocket()
	}
	group := cfg.API.Group
	if group == "" {
		group = defaultAPIGroup()
	}

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.Printf("secon daemon starting (config %s, socket %s)", *cfgPath, *socket)

	// 先にソケットを確保する。別のデーモンが動いていたら、接続を始める前にここで終わる
	ln, err := api.Listen(*socket, group)
	if err != nil {
		return err
	}
	defer os.Remove(*socket)

	nic.CleanupStale(log.Printf) // 前回の異常終了で残った DNS 設定など
	m := engine.NewManager()
	defer m.Close()
	store := &configStore{path: *cfgPath, cfg: cfg, m: m}
	m.Apply(cfg)

	quit := make(chan struct{})
	var once sync.Once
	shutdown := func() { once.Do(func() { stopDaemon(quit) }) }
	srv := &api.Server{Manager: m, Store: store, Reload: store.reload, Shutdown: shutdown}
	go func() {
		if err := srv.Serve(ln); err != nil {
			log.Printf("api: %v", err)
		}
	}()
	defer srv.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	for {
		select {
		case <-ctx.Done():
			log.Printf("shutting down")
			return nil
		case <-quit:
			log.Printf("shutting down (requested via API)")
			return nil
		case <-hup:
			if err := srv.Reload(); err != nil {
				log.Printf("reload: %v", err)
			}
		}
	}
}

// stopDaemon は API からの停止要求を処理する。launchd / systemd 管理下ならそちらに止めてもらい
// (自分で終了すると KeepAlive / Restart で起動し直される)、そうでなければ quit を閉じて終了する。
func stopDaemon(quit chan struct{}) {
	if service.Managed() {
		err := service.StopSelf(os.Geteuid() != 0)
		if err == nil {
			log.Printf("stop requested via API; asking the service manager to stop")
			return
		}
		log.Printf("stop via service manager: %v (exiting instead)", err)
	}
	close(quit)
}

// configStore は設定ファイルとデーモンの状態を同期させる。
type configStore struct {
	path string
	m    *engine.Manager

	mu  sync.Mutex
	cfg *config.Config
}

// reload は設定ファイルを読み直して反映する。保存 (update) と同じロックの中で行い、
// 読み込みと反映の間に保存が割り込んで古い設定に戻ることを防ぐ。
func (s *configStore) reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := config.Load(s.path)
	if err != nil {
		return err
	}
	s.cfg = c
	s.m.Apply(c)
	log.Printf("config reloaded")
	return nil
}

// Profiles はパスワードを伏せた接続設定の一覧 (API で返す)。
func (s *configStore) Profiles() []config.Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]config.Profile, 0, len(s.cfg.Profiles))
	for _, p := range s.cfg.Profiles {
		out = append(out, p.Redacted())
	}
	return out
}

// update は設定ファイルを読み直したうえで変更し、検証・保存・反映する。
// メモリ上の設定から書き出すと、手で編集してまだ reload していない内容を消してしまうため。
func (s *configStore) update(f func(c *config.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, err := config.Load(s.path)
	if err != nil {
		return i18n.Errorf("cfg.fileError", err)
	}
	if err := f(next); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if err := config.Save(s.path, next); err != nil {
		return err
	}
	s.cfg = next
	s.m.Apply(next)
	return nil
}

func (s *configStore) PutProfile(oldName string, p config.Profile) error {
	return s.update(func(c *config.Config) error {
		if oldName == "" {
			p.KeepPasswords(nil)
			c.Profiles = append(c.Profiles, p)
			return nil
		}
		for i := range c.Profiles {
			if c.Profiles[i].Name == oldName {
				p.KeepPasswords(&c.Profiles[i])
				c.Profiles[i] = p
				return nil
			}
		}
		return fmt.Errorf("%w: %s", engine.ErrNoProfile, oldName)
	})
}

func (s *configStore) DeleteProfile(name string) error {
	return s.update(func(c *config.Config) error {
		for i := range c.Profiles {
			if c.Profiles[i].Name == name {
				c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%w: %s", engine.ErrNoProfile, name)
	})
}

// defaultAPIGroup は root デーモンのソケットを操作できるグループの既定値。
func defaultAPIGroup() string {
	if os.Geteuid() != 0 {
		return ""
	}
	candidates := []string{"secon", "sudo", "wheel"}
	if runtime.GOOS == "darwin" {
		candidates = []string{"admin"}
	}
	for _, g := range candidates {
		if _, err := user.LookupGroup(g); err == nil {
			return g
		}
	}
	return ""
}
