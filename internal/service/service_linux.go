package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// Installed は登録済み (ユニットファイルがある) かを返す。
func Installed(user bool) bool {
	_, err := os.Stat(unitPath(user))
	return err == nil
}

// Start は登録済みのデーモンを起動する。
func Start(user bool) error { return systemctl(user, "start", unitName) }

// Managed はこのプロセスが systemd のユニット (secon.service) として動いているかを返す。
func Managed() bool {
	b, _ := os.ReadFile("/proc/self/cgroup")
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasSuffix(line, "/"+unitName) {
			return true
		}
	}
	return false
}

// StopSelf は systemd に自分を止めてもらう (Restart=always で起動し直されないように)。
// 有効化は残すので次回起動時には動く。完了を待つと自分が止められて戻れないので --no-block にする。
func StopSelf(user bool) error {
	return systemctl(user, "--no-block", "stop", unitName)
}

// StartSystem はシステムデーモンを pkexec の認証付きで起動する (prompt は使わない)。
func StartSystem(prompt string) error {
	out, err := exec.Command("pkexec", "systemctl", "start", unitName).CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 126 { // 認証ダイアログが閉じられた
		return ErrCanceled
	}
	if err != nil {
		return fmt.Errorf("pkexec systemctl start: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
