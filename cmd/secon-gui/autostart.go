package main

import (
	"os"
	"path/filepath"
)

// autostartFlag は自動起動時に付ける引数 (このときはウィンドウを出さずトレイに常駐するだけ)。
const autostartFlag = "-autostart"

// guiExecutable は自動起動に登録する secon-gui の実体のパス。
func guiExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
