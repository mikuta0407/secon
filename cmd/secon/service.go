package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/i18n"
	"github.com/mikuta0407/secon/internal/service"
)

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
		fmt.Println(i18n.T("cli.removed", path))
		return nil
	}

	if _, err := os.Stat(cfg); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(cfg, []byte(i18n.T("cli.sampleConfig")), 0o600); err != nil {
			return err
		}
		fmt.Println(i18n.T("cli.createdConfig", cfg))
	}
	path, err := service.Install(o)
	if err != nil {
		return err
	}
	fmt.Println(i18n.T("cli.installed", path))
	return nil
}
