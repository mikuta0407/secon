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
)

const (
	ModeSocks = "socks"
	ModeNIC   = "nic"
)

// Config は設定ファイル全体。
type Config struct {
	API      API       `toml:"api"`
	Profiles []Profile `toml:"profile"`
}

// API はデーモンの制御用ソケット設定。
type API struct {
	Socket string `toml:"socket"` // 空なら既定
	Group  string `toml:"group"`  // ソケットを操作できるグループ (root 実行時)
}

// Profile は 1 つの VPN 接続設定。
type Profile struct {
	Name        string `toml:"name"`
	Server      string `toml:"server"` // host:port (port 省略時 443)
	Hub         string `toml:"hub"`
	User        string `toml:"user"`
	Password    string `toml:"password"` // 空なら匿名認証
	Mode        string `toml:"mode"`     // "socks" | "nic"
	AutoConnect bool   `toml:"auto_connect"`

	// サーバ証明書の検証。CertSHA256 があればピン留め、Insecure なら検証しない、どちらも無ければ OS の信頼ストア
	CertSHA256 string `toml:"cert_sha256"`
	Insecure   bool   `toml:"insecure_skip_verify"`

	Proxy string `toml:"proxy"` // http://[user:pass@]host:port

	Socks    Socks     `toml:"socks"`
	NIC      NIC       `toml:"nic"`
	Forwards []Forward `toml:"forward"`
}

type Socks struct {
	Listen   string `toml:"listen"` // 既定 127.0.0.1:1080
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type NIC struct {
	DefaultGateway bool     `toml:"default_gateway"`
	Routes         []string `toml:"routes"`
	DNS            bool     `toml:"dns"`         // VPN 側 DNS を OS に設定する
	DNSDomains     []string `toml:"dns_domains"` // VPN 側 DNS で解決するドメイン (省略時は DHCP のドメイン名)
}

type Forward struct {
	Listen string `toml:"listen"`
	Target string `toml:"target"`
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
	seen := map[string]bool{}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if p.Name == "" {
			return fmt.Errorf("profile #%d: name is required", i+1)
		}
		if seen[p.Name] {
			return fmt.Errorf("profile %q: duplicate name", p.Name)
		}
		seen[p.Name] = true
		if err := p.normalize(); err != nil {
			return fmt.Errorf("profile %q: %w", p.Name, err)
		}
	}
	return nil
}

func (p *Profile) normalize() error {
	if p.Server == "" {
		return errors.New("server is required")
	}
	if _, _, err := net.SplitHostPort(p.Server); err != nil {
		p.Server = net.JoinHostPort(p.Server, "443")
	}
	if p.Hub == "" {
		return errors.New("hub is required")
	}
	if p.User == "" {
		return errors.New("user is required")
	}
	switch p.Mode {
	case "":
		p.Mode = ModeSocks
	case ModeSocks, ModeNIC:
	default:
		return fmt.Errorf("unknown mode %q", p.Mode)
	}
	p.CertSHA256 = strings.ToLower(strings.ReplaceAll(p.CertSHA256, ":", ""))
	if p.Proxy != "" {
		u, err := url.Parse(p.Proxy)
		if err != nil || u.Scheme != "http" || u.Host == "" {
			return fmt.Errorf("invalid proxy %q (expected http://host:port)", p.Proxy)
		}
	}
	if p.Mode == ModeSocks && p.Socks.Listen == "" {
		p.Socks.Listen = "127.0.0.1:1080"
	}
	for _, r := range p.NIC.Routes {
		if _, err := netip.ParsePrefix(r); err != nil {
			return fmt.Errorf("invalid route %q", r)
		}
	}
	for _, f := range p.Forwards {
		if _, _, err := net.SplitHostPort(f.Listen); err != nil {
			return fmt.Errorf("invalid forward listen %q", f.Listen)
		}
		if _, _, err := net.SplitHostPort(f.Target); err != nil {
			return fmt.Errorf("invalid forward target %q", f.Target)
		}
	}
	return nil
}

// ProxyURL はプロキシ URL (未設定なら nil)。
func (p *Profile) ProxyURL() *url.URL {
	if p.Proxy == "" {
		return nil
	}
	u, _ := url.Parse(p.Proxy)
	return u
}
