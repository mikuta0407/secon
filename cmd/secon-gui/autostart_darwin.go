package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

const autostartLabel = "io.github.mikuta0407.secon-gui"

func autostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", autostartLabel+".plist")
}

func autostartEnabled() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

// setAutostart はログイン時に起動する LaunchAgent を作成・削除する (次回ログインから有効)。
// .app 内から起動されていれば open でアプリとして起動する (Gatekeeper・LaunchServices 的に自然なため)。
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
	args := []string{exe, autostartFlag}
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		args = []string{"/usr/bin/open", "-a", exe[:i+4], "--args", autostartFlag}
	}
	var b strings.Builder
	for _, a := range args {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", html.EscapeString(a))
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>RunAtLoad</key><true/>
	<key>LimitLoadToSessionType</key><string>Aqua</string>
	<key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, autostartLabel, b.String())
	if err := os.MkdirAll(filepath.Dir(autostartPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(autostartPath(), []byte(plist), 0o644)
}
