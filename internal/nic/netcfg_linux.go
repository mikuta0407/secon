package nic

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func linkSetup(name string, mac net.HardwareAddr, mtu int) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	if err := netlink.LinkSetHardwareAddr(link, mac); err != nil {
		return fmt.Errorf("set mac: %w", err)
	}
	if err := netlink.LinkSetMTU(link, mtu); err != nil {
		return fmt.Errorf("set mtu: %w", err)
	}
	return netlink.LinkSetUp(link)
}

func ipNet(p netip.Prefix) *net.IPNet {
	return &net.IPNet{IP: p.Addr().AsSlice(), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())}
}

// Apply は dev にアドレス・経路・DNS を設定し、元に戻す関数を返す。
// 途中で失敗した場合はそこまでの設定を戻してからエラーを返す。
func Apply(dev Device, s Settings, logf func(string, ...any)) (Undo, error) {
	var undos []func()
	undo := func() {
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}
	fail := func(err error) (Undo, error) { undo(); return nil, err }

	link, err := netlink.LinkByName(dev.Name())
	if err != nil {
		return nil, err
	}
	if s.MTU >= 576 {
		netlink.LinkSetMTU(link, s.MTU)
	}
	addr := &netlink.Addr{IPNet: ipNet(s.Address)}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fail(fmt.Errorf("set address: %w", err))
	}
	undos = append(undos, func() { netlink.AddrDel(link, addr) })

	// 既存の経路は上書きしない (切断時に消すと元の経路が失われるため)。同じ宛先が既にあればそのまま使う
	addRoute := func(r *netlink.Route) error {
		if err := netlink.RouteAdd(r); err != nil {
			if errors.Is(err, unix.EEXIST) {
				logf("route %s already exists; leaving it as is", r.Dst)
				return nil
			}
			return fmt.Errorf("add route %s: %w", r.Dst, err)
		}
		undos = append(undos, func() { netlink.RouteDel(r) })
		return nil
	}

	if s.DefaultGateway {
		if !s.Gateway.IsValid() {
			return fail(errors.New("default_gateway: DHCP did not provide a router"))
		}
		// VPN サーバへの経路を元の経路に固定してから 0/1 と 128/1 を奪う
		for _, b := range s.Bypass {
			rs, err := netlink.RouteGet(b.AsSlice())
			if err != nil || len(rs) == 0 {
				return fail(fmt.Errorf("lookup route to %s: %v", b, err))
			}
			orig := rs[0]
			if orig.Gw == nil {
				continue // 直結のサブネット上にある (より細かい経路があるので 0/1・128/1 に取られない)
			}
			bypass := &netlink.Route{Dst: ipNet(netip.PrefixFrom(b, 32)), Gw: orig.Gw, LinkIndex: orig.LinkIndex}
			if err := addRoute(bypass); err != nil {
				return fail(err)
			}
		}
		for _, p := range halfDefaults {
			if err := addRoute(&netlink.Route{Dst: ipNet(p), Gw: s.Gateway.AsSlice(), LinkIndex: link.Attrs().Index}); err != nil {
				return fail(err)
			}
		}
	}
	for _, p := range s.Routes {
		r := &netlink.Route{Dst: ipNet(p), LinkIndex: link.Attrs().Index}
		if s.Gateway.IsValid() && viaGateway(s.Address, p) {
			r.Gw = s.Gateway.AsSlice()
		}
		if err := addRoute(r); err != nil {
			return fail(err)
		}
	}

	if len(s.DNS) > 0 {
		if err := setDNS(dev.Name(), s.DNS, s.DNSDomains, s.DefaultGateway); err != nil {
			logf("dns: %v (skipped)", err)
		} else {
			undos = append(undos, func() { exec.Command("resolvectl", "revert", dev.Name()).Run() })
		}
	}
	return undo, nil
}

// setDNS は systemd-resolved にインターフェース単位の DNS を設定する。
// defaultRoute のときは全ドメインの問い合わせをこの NIC に向ける。
func setDNS(ifname string, servers []netip.Addr, domains []string, defaultRoute bool) error {
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return errors.New("resolvectl not found")
	}
	args := []string{"dns", ifname}
	for _, s := range servers {
		args = append(args, s.String())
	}
	if out, err := exec.Command("resolvectl", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("resolvectl dns: %v: %s", err, out)
	}
	dom := []string{"domain", ifname}
	for _, d := range domains {
		dom = append(dom, "~"+d)
	}
	if defaultRoute {
		dom = append(dom, "~.")
	}
	if len(dom) > 2 {
		if out, err := exec.Command("resolvectl", dom...).CombinedOutput(); err != nil {
			return fmt.Errorf("resolvectl domain: %v: %s", err, out)
		}
	}
	return nil
}

// CleanupStale は前回の異常終了で残った設定を消す (Linux は TAP と共に消えるので何もしない)。
func CleanupStale(logf func(string, ...any)) {}
