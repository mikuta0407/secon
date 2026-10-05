// Package nic は OS の仮想 NIC (Linux: TAP, macOS: utun) とアドレス・経路・DNS 設定を扱う。
package nic

import (
	"net"
	"net/netip"
	"regexp"
	"strings"
)

// Device は Ethernet フレームを送受信する仮想 NIC。
// macOS の utun のように L3 しか扱えない OS では実装側で L2 をエミュレートする。
type Device interface {
	Name() string
	ReadFrame([]byte) (int, error)
	WriteFrame([]byte) error
	Close() error
}

// Settings は VPN 接続後に OS へ設定する内容。
type Settings struct {
	Address        netip.Prefix
	Gateway        netip.Addr // VPN 側ゲートウェイ (DHCP の router)
	MTU            int
	Routes         []netip.Prefix // VPN 経由にする宛先
	DefaultGateway bool           // VPN をデフォルトゲートウェイにする
	Bypass         []netip.Addr   // DefaultGateway 時に元の経路で送る宛先 (VPN サーバとプロキシ)
	DNS            []netip.Addr   // 空なら DNS を設定しない
	DNSDomains     []string       // この DNS で解決するドメイン (DefaultGateway 時は全ドメイン)
}

// Undo は Apply した設定を元に戻す。
type Undo func()

var invalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// InterfaceName はプロファイル名から Linux のインターフェース名 (15 文字以内) を作る。
func InterfaceName(profile string) string {
	n := "secon-" + invalidChars.ReplaceAllString(strings.ToLower(profile), "")
	if len(n) > 15 {
		n = n[:15]
	}
	return n
}

// halfDefaults は元のデフォルトルートを消さずに全トラフィックを奪う 2 経路。
var halfDefaults = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/1"),
	netip.MustParsePrefix("128.0.0.0/1"),
}

// viaGateway は経路 p を VPN 側ゲートウェイ経由にすべきか。
// p が自分のサブネットに収まる (サブネット以下の広さ) ならゲートウェイ不要 (直接届く)。
func viaGateway(addr netip.Prefix, p netip.Prefix) bool {
	return !(addr.Masked().Contains(p.Addr()) && p.Bits() >= addr.Bits())
}

// HostAddr は net.Addr から IP を取り出す。
func HostAddr(a net.Addr) netip.Addr {
	if ap, err := netip.ParseAddrPort(a.String()); err == nil {
		return ap.Addr().Unmap()
	}
	return netip.Addr{}
}
