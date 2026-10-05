package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"os/user"
	"runtime"
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
	srv := &api.Server{Manager: m, Reload: func() error {
		c, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		m.Apply(c)
		log.Printf("config reloaded")
		return nil
	}}
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
