package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	c := &Config{Profiles: []Profile{
		{Name: "a", Server: "vpn.example.com", Hub: "VPN", User: "u", Password: "p", Mode: ModeNIC,
			NIC:      NIC{Routes: []string{"10.0.0.0/8"}, DNS: true},
			Forwards: []Forward{{Listen: "127.0.0.1:13389", Target: "10.0.0.5:3389"}}},
		{Name: "b", Server: "vpn.example.com:5555", Hub: "VPN", User: "u"},
	}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "[profile.socks]") && strings.Count(string(b), "[profile.socks]") != 1 {
		t.Errorf("empty socks section should be omitted:\n%s", b)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Profiles, c.Profiles) {
		t.Errorf("round trip mismatch\n got %+v\nwant %+v\nfile:\n%s", got.Profiles, c.Profiles, b)
	}
	if c.Profiles[0].Server != "vpn.example.com:443" || c.Profiles[1].Mode != ModeSocks || c.Profiles[1].Socks.Listen != "127.0.0.1:1080" {
		t.Errorf("defaults not applied: %+v", c.Profiles)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestValidateErrors(t *testing.T) {
	for _, p := range []Profile{
		{Name: "x", Hub: "h", User: "u"},
		{Name: "x", Server: "s", Hub: "h", User: "u", Mode: "bogus"},
		{Name: "x", Server: "s", Hub: "h", User: "u", NIC: NIC{Routes: []string{"10.0.0.0"}}},
		{Name: "x", Server: "s", Hub: "h", User: "u", Proxy: "socks5://p:1"},
	} {
		if err := p.Normalize(); err == nil {
			t.Errorf("expected error for %+v", p)
		}
	}
	c := &Config{Profiles: []Profile{{Name: "x", Server: "s", Hub: "h", User: "u"}, {Name: "x", Server: "s", Hub: "h", User: "u"}}}
	if err := c.Validate(); err == nil {
		t.Error("expected duplicate name error")
	}
}

func TestStaticValidation(t *testing.T) {
	base := Profile{Name: "x", Server: "s", Hub: "h", User: "u"}
	ok := []Static{
		{},
		{Address: "10.0.0.50/24"},
		{Address: "10.0.0.50/24", Gateway: "10.0.0.1", DNS: []string{"10.0.0.1", "8.8.8.8"}},
	}
	for _, s := range ok {
		p := base
		p.Static = s
		if err := p.Normalize(); err != nil {
			t.Errorf("%+v: unexpected error %v", s, err)
		}
	}
	bad := []Static{
		{Gateway: "10.0.0.1"},                          // アドレス無し
		{Address: "10.0.0.50"},                         // プレフィックス無し
		{Address: "10.0.0.0/24"},                       // ネットワークアドレス
		{Address: "10.0.0.50/24", Gateway: "10.1.0.1"}, // サブネット外
		{Address: "10.0.0.50/24", DNS: []string{"x"}},
		{Address: "fd00::5/64"},
	}
	for _, s := range bad {
		p := base
		p.Static = s
		if err := p.Normalize(); err == nil {
			t.Errorf("%+v: expected error", s)
		}
	}
}

func TestValidateRejectsUnsafeValues(t *testing.T) {
	base := Profile{Name: "x", Server: "s", Hub: "h", User: "u"}
	bad := []func(p *Profile){
		func(p *Profile) { p.NIC.DNSDomains = []string{"corp\nremove State:/Network/Global/IPv4"} }, // scutil への注入
		func(p *Profile) { p.NIC.DNSDomains = []string{"a b"} },
		func(p *Profile) { p.NIC.Routes = []string{"fd00::/8"} }, // IPv6 は未対応
		func(p *Profile) { p.Socks.Password = "pw" },             // ユーザ名なしは認証が無効になる
		func(p *Profile) { p.Proxy = "http://proxy.example.com" },
	}
	for i, f := range bad {
		p := base
		f(&p)
		if err := p.Normalize(); err == nil {
			t.Errorf("case %d: expected error for %+v", i, p)
		}
	}
	ok := base
	ok.NIC.DNSDomains = []string{"corp.example", "example.com."}
	ok.Proxy = "http://user:pass@proxy.example.com:8080"
	if err := ok.Normalize(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRedactAndKeepPasswords(t *testing.T) {
	old := Profile{Name: "x", Password: "secret", Socks: Socks{Username: "s", Password: "spw"}}
	r := old.Redacted()
	if r.Password != "" || !r.PasswordSet || r.Socks.Password != "" || !r.Socks.PasswordSet {
		t.Fatalf("not redacted: %+v", r)
	}
	// GUI が空欄のまま送り返したら保存済みを引き継ぐ
	r.KeepPasswords(&old)
	if r.Password != "secret" || r.Socks.Password != "spw" || r.PasswordSet || r.Socks.PasswordSet {
		t.Errorf("keep failed: %+v", r)
	}
	// 新しいパスワードを入れたらそちらを使う
	n := Profile{Name: "x", Password: "new", PasswordSet: false}
	n.KeepPasswords(&old)
	if n.Password != "new" {
		t.Errorf("new password replaced: %+v", n)
	}
}
