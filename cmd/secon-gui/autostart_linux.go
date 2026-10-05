package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func autostartPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "autostart", "secon-gui.desktop")
}

func autostartEnabled() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

// setAutostart は XDG autostart の .desktop を作成・削除する (次回ログインから有効)。
func setAutostart(on bool) error {
	if !on {
		err := os.Remove(autostartPath())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := guiExecutable()
	if err != nil {
		return err
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=secon
Comment=SoftEther VPN compatible client
Exec="%s" %s
Icon=network-vpn
Terminal=false
X-GNOME-Autostart-enabled=true
`, exe, autostartFlag)
	if err := os.MkdirAll(filepath.Dir(autostartPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(autostartPath(), []byte(desktop), 0o644)
}
