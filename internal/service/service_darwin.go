package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	run("launchctl", "bootout", domain(o.User)+"/"+Label) // 登録済みなら一旦外す
	if err := writeFile(path, plist); err != nil {
		return "", err
	}
	if err := run("launchctl", "bootstrap", domain(o.User), path); err != nil {
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
	run("launchctl", "bootout", domain(o.User)+"/"+Label)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}
