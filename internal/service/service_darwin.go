package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// bootout は登録を外し、launchd から消えるまで待つ (最大 10 秒)。
func bootout(user bool) {
	target := domain(user) + "/" + Label
	if run("launchctl", "bootout", target) != nil {
		return // 未登録
	}
	for i := 0; i < 50; i++ {
		if exec.Command("launchctl", "print", target).Run() != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
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
	if err := run("launchctl", "bootstrap", domain(o.User), path); err != nil {
		// 停止処理が残っていると "Bootstrap failed: 5" になることがあるので少し待って再試行する
		time.Sleep(2 * time.Second)
		if err := run("launchctl", "bootstrap", domain(o.User), path); err != nil {
			return "", err
		}
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
