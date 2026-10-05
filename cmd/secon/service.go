package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/service"
)

const sampleConfig = `# secon 設定ファイル。編集後は 'secon reload' で反映する。
#
# [[profile]]
# name = "office"
# server = "vpn.example.com:443"
# hub = "VPN"
# user = "alice"
# password = "secret"
# mode = "nic"                # "nic" (仮想 NIC, root 必要) または "socks"
# auto_connect = true
# cert_sha256 = ""            # サーバ証明書の SHA-256 (自己署名証明書のピン留め)
# proxy = ""                  # http://user:pass@proxy:8080
# nic = { routes = ["10.0.0.0/8"], default_gateway = false, dns = true }
# socks = { listen = "127.0.0.1:1080" }
#
# [[profile.forward]]
# listen = "127.0.0.1:13389"
# target = "10.0.0.5:3389"
`

func runService(args []string) error {
	if len(args) < 1 || (args[0] != "install" && args[0] != "uninstall") {
		return errors.New("usage: secon service install|uninstall [--user] [-config path]")
	}
	fs := flag.NewFlagSet("service "+args[0], flag.ExitOnError)
	user := fs.Bool("user", false, "ユーザ単位で登録する (SOCKS モードのみ・root 不要)")
	cfgPath := fs.String("config", config.DefaultPath(), "デーモンに渡す設定ファイル")
	fs.Parse(args[1:])

	exe, err := service.Executable()
	if err != nil {
		return err
	}
	cfg, err := filepath.Abs(*cfgPath)
	if err != nil {
		return err
	}
	o := service.Options{User: *user, Executable: exe, Config: cfg}

	if args[0] == "uninstall" {
		path, err := service.Uninstall(o)
		if err != nil {
			return err
		}
		fmt.Printf("removed %s\n", path)
		return nil
	}

	if _, err := os.Stat(cfg); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(cfg, []byte(sampleConfig), 0o600); err != nil {
			return err
		}
		fmt.Printf("created %s (edit it, then run 'secon reload')\n", cfg)
	}
	path, err := service.Install(o)
	if err != nil {
		return err
	}
	fmt.Printf("installed %s\n", path)
	return nil
}
