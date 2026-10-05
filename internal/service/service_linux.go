package service

import (
	"fmt"
	"os"
	"path/filepath"
)

const unitName = "secon.service"

func unitPath(user bool) string {
	if user {
		dir, err := os.UserConfigDir()
		if err != nil {
			dir = filepath.Join(os.Getenv("HOME"), ".config")
		}
		return filepath.Join(dir, "systemd", "user", unitName)
	}
	return filepath.Join("/etc/systemd/system", unitName)
}

func systemctl(user bool, args ...string) error {
	if user {
		args = append([]string{"--user"}, args...)
	}
	return run("systemctl", args...)
}

// Install は systemd に登録して起動する。
func Install(o Options) (string, error) {
	if err := checkRoot(o.User); err != nil {
		return "", err
	}
	target := "multi-user.target"
	if o.User {
		target = "default.target"
	}
	unit := fmt.Sprintf(`[Unit]
Description=secon SoftEther VPN client
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=%s daemon -config %s
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=3

[Install]
WantedBy=%s
`, o.Executable, o.Config, target)
	path := unitPath(o.User)
	if err := writeFile(path, unit); err != nil {
		return "", err
	}
	if err := systemctl(o.User, "daemon-reload"); err != nil {
		return "", err
	}
	if err := systemctl(o.User, "enable", "--now", unitName); err != nil {
		return "", err
	}
	// 既に動いていた場合に新しい設定で起動し直す
	return path, systemctl(o.User, "restart", unitName)
}

// Uninstall は systemd から外して停止する。
func Uninstall(o Options) (string, error) {
	if err := checkRoot(o.User); err != nil {
		return "", err
	}
	path := unitPath(o.User)
	systemctl(o.User, "disable", "--now", unitName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, systemctl(o.User, "daemon-reload")
}
