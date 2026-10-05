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
)

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "設定ファイル")
	socket := fs.String("socket", "", "制御ソケット (既定: 設定ファイルの api.socket か "+api.DefaultSocket()+")")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
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

	m := engine.NewManager()
	defer m.Close()
	m.Apply(cfg)

	ln, err := api.Listen(*socket, group)
	if err != nil {
		return err
	}
	defer os.Remove(*socket)
	store := &configStore{path: *cfgPath, cfg: cfg, m: m}
	srv := &api.Server{Manager: m, Store: store, Reload: store.reload}
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
		case <-hup:
			if err := srv.Reload(); err != nil {
				log.Printf("reload: %v", err)
			}
		}
	}
}

// configStore は設定ファイルとデーモンの状態を同期させる。
type configStore struct {
	path string
	m    *engine.Manager

	mu  sync.Mutex
	cfg *config.Config
}

func (s *configStore) reload() error {
	c, err := config.Load(s.path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = c
	s.mu.Unlock()
	s.m.Apply(c)
	log.Printf("config reloaded")
	return nil
}

func (s *configStore) Profiles() []config.Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]config.Profile(nil), s.cfg.Profiles...)
}

// update は設定を変更して検証・保存・反映する。
func (s *configStore) update(f func(c *config.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := *s.cfg
	next.Profiles = append([]config.Profile(nil), s.cfg.Profiles...)
	if err := f(&next); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if err := config.Save(s.path, &next); err != nil {
		return err
	}
	s.cfg = &next
	s.m.Apply(&next)
	return nil
}

func (s *configStore) PutProfile(oldName string, p config.Profile) error {
	return s.update(func(c *config.Config) error {
		if oldName == "" {
			c.Profiles = append(c.Profiles, p)
			return nil
		}
		for i := range c.Profiles {
			if c.Profiles[i].Name == oldName {
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
