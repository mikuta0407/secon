package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/engine"
)

// API から保存しても、手で追加してまだ reload していない接続設定が消えないこと。
// また、伏せたパスワードを空欄で送り返したら保存済みのものが残ること。
func TestConfigStoreUpdateKeepsManualEditsAndPasswords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	initial := "[[profile]]\nname = \"a\"\nserver = \"vpn.example.com:443\"\nhub = \"VPN\"\nuser = \"u\"\npassword = \"secret\"\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := engine.NewManager()
	m.Logf = t.Logf
	defer m.Close()
	s := &configStore{path: path, cfg: cfg, m: m}

	// 手で追記 (reload しない)
	manual := initial + "\n[[profile]]\nname = \"manual\"\nserver = \"vpn2.example.com:443\"\nhub = \"VPN\"\nuser = \"u\"\n"
	if err := os.WriteFile(path, []byte(manual), 0o600); err != nil {
		t.Fatal(err)
	}

	// GUI と同じく、伏せられた状態で受け取ってユーザ名だけ変えて保存
	got := s.Profiles()
	if len(got) != 1 || got[0].Password != "" || !got[0].PasswordSet {
		t.Fatalf("profiles not redacted: %+v", got)
	}
	p := got[0]
	p.User = "changed"
	if err := s.PutProfile("a", p); err != nil {
		t.Fatal(err)
	}

	after, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Profiles) != 2 || after.Profiles[1].Name != "manual" {
		t.Fatalf("manual edit lost: %+v", after.Profiles)
	}
	if after.Profiles[0].User != "changed" || after.Profiles[0].Password != "secret" {
		t.Errorf("profile a = %+v", after.Profiles[0])
	}
}
