package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func plistPath(user bool) string {
	if user {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
	}
	return filepath.Join("/Library/LaunchDaemons", Label+".plist")
}

func domain(user bool) string {
	if user {
		return "gui/" + strconv.Itoa(os.Getuid())
	}
	return "system"
}

func logPath(user bool) string {
	if user {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "secon.log")
	}
	return "/var/log/secon.log"
}

func loaded(user bool) bool {
	return exec.Command("launchctl", "print", domain(user)+"/"+Label).Run() == nil
}

// bootout は登録を外し、launchd から消えるまで待つ (最大 10 秒)。未登録なら何もしない。
func bootout(user bool) error {
	if !loaded(user) {
		return nil
	}
	if err := run("launchctl", "bootout", domain(user)+"/"+Label); err != nil {
		return err
	}
	for i := 0; i < 50 && loaded(user); i++ {
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func bootstrap(user bool) error {
	if err := run("launchctl", "bootstrap", domain(user), plistPath(user)); err != nil {
		// 停止処理が残っていると "Bootstrap failed: 5" になることがあるので少し待って再試行する
		time.Sleep(2 * time.Second)
		return run("launchctl", "bootstrap", domain(user), plistPath(user))
	}
	return nil
}

// Install は launchd に登録して起動する。
func Install(o Options) (string, error) {
	if err := checkRoot(o.User); err != nil {
		return "", err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
		<string>-config</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, Label, o.Executable, o.Config, logPath(o.User), logPath(o.User))
	path := plistPath(o.User)
	bootout(o.User) // 登録済みなら一旦外す
	if err := writeFile(path, plist); err != nil {
		return "", err
	}
	if err := bootstrap(o.User); err != nil {
		return "", err
	}
	return path, nil
}

// Uninstall は launchd から外して停止する。
func Uninstall(o Options) (string, error) {
	if err := checkRoot(o.User); err != nil {
		return "", err
	}
	path := plistPath(o.User)
	bootout(o.User)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

// Installed は登録済み (plist がある) かを返す。
func Installed(user bool) bool {
	_, err := os.Stat(plistPath(user))
	return err == nil
}

// Start は登録済みのデーモンを起動する (読み込み済みなら何もしない)。
func Start(user bool) error {
	if loaded(user) {
		return nil
	}
	return bootstrap(user)
}

// Managed はこのプロセスが launchd に登録されたデーモンとして動いているかを返す。
func Managed() bool {
	return os.Getenv("XPC_SERVICE_NAME") == Label
}

// StopSelf は launchd から自分の登録を外す (KeepAlive で起動し直されないように)。
// launchd が SIGTERM を送ってくるので、終了処理はいつもどおり行われる。plist は残すので次回起動時には動く。
// launchctl がこのプロセスと一緒に止められないよう、別セッションで動かす。
func StopSelf(user bool) error {
	cmd := exec.Command("launchctl", "bootout", domain(user)+"/"+Label)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// StartSystem はシステムデーモンを管理者認証付きで起動する (読み込み済みなら何もしない)。
// prompt は認証ダイアログに出す説明。
func StartSystem(prompt string) error {
	if loaded(false) {
		return nil
	}
	// 停止直後は "Bootstrap failed: 5" になることがあるので、認証 1 回のまま少し待って再試行する
	cmd := fmt.Sprintf("/bin/launchctl bootstrap system %[1]s || (sleep 2; /bin/launchctl bootstrap system %[1]s)", plistPath(false))
	script := fmt.Sprintf("do shell script %s with prompt %s with administrator privileges", appleScriptString(cmd), appleScriptString(prompt))
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "(-128)") { // ユーザがキャンセルした
			return ErrCanceled
		}
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
