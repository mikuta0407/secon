// Package config は設定ファイル (TOML) を扱う。
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/mikuta0407/secon/internal/i18n"
)

const (
	ModeSocks = "socks"
	ModeNIC   = "nic"
)

// Config は設定ファイル全体。
type Config struct {
	API      API       `toml:"api,omitempty" json:"api,omitempty"`
	Profiles []Profile `toml:"profile" json:"profile"`
}

// API はデーモンの制御用ソケット設定。
type API struct {
	Socket string `toml:"socket,omitempty" json:"socket,omitempty"` // 空なら既定
	Group  string `toml:"group,omitempty" json:"group,omitempty"`   // ソケットを操作できるグループ (root 実行時)
}

// Profile は 1 つの VPN 接続設定。
type Profile struct {
	Name        string `toml:"name" json:"name"`
	Server      string `toml:"server" json:"server"` // host:port (port 省略時 443)
	Hub         string `toml:"hub" json:"hub"`
	User        string `toml:"user" json:"user"`
	Password    string `toml:"password,omitempty" json:"password,omitempty"` // 空なら匿名認証
	Mode        string `toml:"mode,omitempty" json:"mode,omitempty"`         // "socks" | "nic"
	AutoConnect bool   `toml:"auto_connect,omitempty" json:"auto_connect,omitempty"`

	// サーバ証明書の検証。CertSHA256 があればピン留め、Insecure なら検証しない、どちらも無ければ OS の信頼ストア
	CertSHA256 string `toml:"cert_sha256,omitempty" json:"cert_sha256,omitempty"`
	Insecure   bool   `toml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`

	Proxy string `toml:"proxy,omitempty" json:"proxy,omitempty"` // http://[user:pass@]host:port

	Static   Static    `toml:"static,omitempty" json:"static,omitempty"` // DHCP を使わずに固定アドレスを使う
	Socks    Socks     `toml:"socks,omitempty" json:"socks,omitempty"`
	NIC      NIC       `toml:"nic,omitempty" json:"nic,omitempty"`
	Forwards []Forward `toml:"forward,omitempty" json:"forward,omitempty"`
}

// Static は DHCP の代わりに使う固定アドレス設定。Address が空なら DHCP。
type Static struct {
	Address string   `toml:"address,omitempty" json:"address,omitempty"` // CIDR (例 10.0.0.50/24)
	Gateway string   `toml:"gateway,omitempty" json:"gateway,omitempty"`
	DNS     []string `toml:"dns,omitempty" json:"dns,omitempty"`
}

// Enabled は固定アドレスが設定されているか。
func (s Static) Enabled() bool { return s.Address != "" }

type Socks struct {
	Listen   string `toml:"listen,omitempty" json:"listen,omitempty"` // 既定 127.0.0.1:1080
	Username string `toml:"username,omitempty" json:"username,omitempty"`
	Password string `toml:"password,omitempty" json:"password,omitempty"`
}

type NIC struct {
	DefaultGateway bool     `toml:"default_gateway,omitempty" json:"default_gateway,omitempty"`
	Routes         []string `toml:"routes,omitempty" json:"routes,omitempty"`
	DNS            bool     `toml:"dns,omitempty" json:"dns,omitempty"`                 // VPN 側 DNS を OS に設定する
	DNSDomains     []string `toml:"dns_domains,omitempty" json:"dns_domains,omitempty"` // VPN 側 DNS で解決するドメイン (省略時は DHCP のドメイン名)
}

type Forward struct {
	Listen string `toml:"listen,omitempty" json:"listen,omitempty"`
	Target string `toml:"target,omitempty" json:"target,omitempty"`
}

func (s Static) validate() error {
	if !s.Enabled() {
		if s.Gateway != "" || len(s.DNS) > 0 {
			return i18n.Errorf("cfg.staticNeedsAddress")
		}
		return nil
	}
	prefix, err := netip.ParsePrefix(s.Address)
	if err != nil || !prefix.Addr().Is4() {
		return i18n.Errorf("cfg.invalidAddress", s.Address)
	}
	if prefix.Addr() == prefix.Masked().Addr() && prefix.Bits() < 31 {
		return i18n.Errorf("cfg.networkAddress", s.Address)
	}
	if s.Gateway != "" {
		gw, err := netip.ParseAddr(s.Gateway)
		if err != nil || !gw.Is4() {
			return i18n.Errorf("cfg.invalidGateway", s.Gateway)
		}
		if !prefix.Masked().Contains(gw) {
			return i18n.Errorf("cfg.gatewayOutside", s.Gateway, s.Address)
		}
	}
	for _, d := range s.DNS {
		if a, err := netip.ParseAddr(d); err != nil || !a.Is4() {
			return i18n.Errorf("cfg.invalidDNS", d)
		}
	}
	return nil
}

// DefaultPath は既定の設定ファイルパス。
func DefaultPath() string {
	if os.Geteuid() == 0 {
		return "/etc/secon/config.toml"
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "secon", "config.toml")
}

// Load は設定ファイルを読み、既定値を補って検証する。ファイルが無ければ空の設定を返す。
func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &c, nil
		}
		return nil, err
	}
	if err := c.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) normalize() error {
	return c.Validate()
}

// Validate は全プロファイルを検証し、既定値を補う。
func (c *Config) Validate() error {
	seen := map[string]bool{}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if p.Name == "" {
			return i18n.Errorf("cfg.profileNameRequired", i+1)
		}
		if seen[p.Name] {
			return i18n.Errorf("cfg.duplicateName", p.Name)
		}
		seen[p.Name] = true
		if err := p.Normalize(); err != nil {
			return i18n.Errorf("cfg.profileError", p.Name, err)
		}
	}
	return nil
}

// Normalize はプロファイルを検証し、既定値を補う。
func (p *Profile) Normalize() error {
	if p.Name == "" {
		return i18n.Errorf("cfg.nameRequired")
	}
	if p.Server == "" {
		return i18n.Errorf("cfg.serverRequired")
	}
	if _, _, err := net.SplitHostPort(p.Server); err != nil {
		p.Server = net.JoinHostPort(p.Server, "443")
	}
	if p.Hub == "" {
		return i18n.Errorf("cfg.hubRequired")
	}
	if p.User == "" {
		return i18n.Errorf("cfg.userRequired")
	}
	switch p.Mode {
	case "":
		p.Mode = ModeSocks
	case ModeSocks, ModeNIC:
	default:
		return i18n.Errorf("cfg.unknownMode", p.Mode)
	}
	p.CertSHA256 = strings.ToLower(strings.ReplaceAll(p.CertSHA256, ":", ""))
	if p.Proxy != "" {
		u, err := url.Parse(p.Proxy)
		if err != nil || u.Scheme != "http" || u.Host == "" {
			return i18n.Errorf("cfg.invalidProxy", p.Proxy)
		}
	}
	if p.Mode == ModeSocks && p.Socks.Listen == "" {
		p.Socks.Listen = "127.0.0.1:1080"
	}
	if err := p.Static.validate(); err != nil {
		return err
	}
	for _, r := range p.NIC.Routes {
		if _, err := netip.ParsePrefix(r); err != nil {
			return i18n.Errorf("cfg.invalidRoute", r)
		}
	}
	for _, f := range p.Forwards {
		if _, _, err := net.SplitHostPort(f.Listen); err != nil {
			return i18n.Errorf("cfg.invalidForwardListen", f.Listen)
		}
		if _, _, err := net.SplitHostPort(f.Target); err != nil {
			return i18n.Errorf("cfg.invalidForwardTarget", f.Target)
		}
	}
	return nil
}

// Save は設定をファイルに書く (0600)。コメントは保持されない。
func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# secon 設定ファイル (secon が書き換えることがあります)\n\n")
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ProxyURL はプロキシ URL (未設定なら nil)。
func (p *Profile) ProxyURL() *url.URL {
	if p.Proxy == "" {
		return nil
	}
	u, _ := url.Parse(p.Proxy)
	return u
}
