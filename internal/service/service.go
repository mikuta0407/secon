// Package service はデーモンを launchd (macOS) / systemd (Linux) に登録する。
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Label は launchd のラベル / systemd のユニット名。
const Label = "io.github.mikuta0407.secon"

// Options は登録内容。
type Options struct {
	User       bool   // true ならユーザ単位 (SOCKS モードのみ・root 不要)
	Executable string // 空なら自動 (Homebrew のシンボリックリンクを優先)
	Config     string // 設定ファイル
}

// Executable は登録に使う secon のパスを返す。
// Homebrew の Cellar 内の実体ではなく、アップグレード後も有効な bin/secon を優先する。
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	if i := strings.Index(exe, "/Cellar/"); i >= 0 {
		link := filepath.Join(exe[:i], "bin", filepath.Base(exe))
		if real, err := filepath.EvalSymlinks(link); err == nil && real == exe {
			return link, nil
		}
	}
	return exe, nil
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func checkRoot(user bool) error {
	if !user && os.Geteuid() != 0 {
		return fmt.Errorf("system service requires root (use sudo, or --user for a SOCKS-only user service)")
	}
	if user && os.Geteuid() == 0 {
		return fmt.Errorf("--user must not be run as root")
	}
	return nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
