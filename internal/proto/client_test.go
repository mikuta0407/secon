package proto

import (
	"bytes"
	"testing"
)

func TestLoginPackPlainPassword(t *testing.T) {
	random := bytes.Repeat([]byte{0x11}, 32)
	p := loginPack(LoginConfig{
		Hub: "HUB", Username: "user", Password: "secret", AuthType: AuthPlainPassword,
	}, random)

	if got := p.GetInt("authtype"); got != AuthPlainPassword {
		t.Errorf("authtype = %d, want %d", got, AuthPlainPassword)
	}
	if pw, ok := p.GetStr("plain_password"); !ok || pw != "secret" {
		t.Errorf("plain_password = %q (ok=%v), want %q", pw, ok, "secret")
	}
	if _, ok := p.GetData("secure_password"); ok {
		t.Error("plain auth must not include secure_password")
	}
}

func TestLoginPackHashedPassword(t *testing.T) {
	random := bytes.Repeat([]byte{0x11}, 32)
	p := loginPack(LoginConfig{
		Hub: "HUB", Username: "user", Password: "secret", AuthType: AuthPassword,
	}, random)

	if got := p.GetInt("authtype"); got != AuthPassword {
		t.Errorf("authtype = %d, want %d", got, AuthPassword)
	}
	if _, ok := p.GetStr("plain_password"); ok {
		t.Error("hashed auth must not include plain_password")
	}
	if _, ok := p.GetData("secure_password"); !ok {
		t.Error("hashed auth must include secure_password")
	}
}
