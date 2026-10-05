//go:build darwin || linux

package main

import (
	"os"
	"strings"
	"testing"
)

// 自動起動の登録・解除 (実際のホームではなく一時ディレクトリで)。
func TestAutostartToggle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir+"/.config")

	if autostartEnabled() {
		t.Fatal("should be disabled initially")
	}
	if err := setAutostart(true); err != nil {
		t.Fatal(err)
	}
	if !autostartEnabled() {
		t.Fatal("not enabled")
	}
	b, err := os.ReadFile(autostartPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(autostartPath(), dir) || !strings.Contains(string(b), autostartFlag) {
		t.Errorf("unexpected entry at %s:\n%s", autostartPath(), b)
	}
	if err := setAutostart(false); err != nil {
		t.Fatal(err)
	}
	if autostartEnabled() {
		t.Fatal("still enabled")
	}
	if err := setAutostart(false); err != nil { // 未登録の解除はエラーにしない
		t.Fatal(err)
	}
}
