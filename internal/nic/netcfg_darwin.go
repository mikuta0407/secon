package nic

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"
)

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return nil
}

func netmask(bits int) string {
	return net.IP(net.CIDRMask(bits, 32)).String()
}

// Apply は utun にアドレス・経路・DNS を設定し、元に戻す関数を返す。
func Apply(dev Device, s Settings, logf func(string, ...any)) (Undo, error) {
	u, ok := dev.(*utun)
	if !ok {
		return nil, errors.New("unexpected device type")
	}
	var undos []func()
	undo := func() {
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}
	fail := func(err error) (Undo, error) { undo(); return nil, err }

	name := u.Name()
	ip := s.Address.Addr().String()
	args := []string{name, "inet", ip, ip, "netmask", netmask(s.Address.Bits())}
	if s.MTU >= 576 {
		args = append(args, "mtu", fmt.Sprint(s.MTU))
	}
	if err := run("ifconfig", append(args, "up")...); err != nil {
		return fail(err)
	}
	undos = append(undos, func() { run("ifconfig", name, "inet", ip, "-alias") })
	u.setIPv4(s.Address, s.Gateway)

	addRoute := func(dst netip.Prefix, gwArgs ...string) error {
		kind := "-net"
		if dst.Bits() == 32 {
			kind = "-host"
		}
		target := dst.String()
		if kind == "-host" {
			target = dst.Addr().String()
		}
		err := run("route", append([]string{"-n", "add", kind, target}, gwArgs...)...)
		if err != nil && strings.Contains(err.Error(), "File exists") {
			// 前回の異常終了で残った経路など。張り替えて使う
			err = run("route", append([]string{"-n", "change", kind, target}, gwArgs...)...)
		}
		if err != nil {
			return err
		}
		undos = append(undos, func() { run("route", "-n", "delete", kind, target) })
		return nil
	}

	// utun は point-to-point なのでサブネット経路を明示する
	if err := addRoute(s.Address.Masked(), "-interface", name); err != nil {
		return fail(err)
	}

	if s.DefaultGateway {
		if !s.Gateway.IsValid() {
			return fail(errors.New("default_gateway: DHCP did not provide a router"))
		}
		for _, b := range s.Bypass {
			gw, err := currentGateway(b)
			if err != nil {
				return fail(err)
			}
			if gw == nil {
				continue // 直結のサブネット上にある (より細かい経路があるので 0/1・128/1 に取られない)
			}
			if err := addRoute(netip.PrefixFrom(b, 32), gw...); err != nil {
				return fail(err)
			}
		}
		for _, p := range halfDefaults {
			if err := addRoute(p, "-interface", name); err != nil {
				return fail(err)
			}
		}
	}
	for _, p := range s.Routes {
		// 次ホップの決定は L2 エミュレーションが行う (サブネット外ならゲートウェイ)
		if err := addRoute(p, "-interface", name); err != nil {
			return fail(err)
		}
	}

	if len(s.DNS) > 0 {
		key := dnsKeyPrefix + name + "/DNS"
		if err := setDNS(key, s.DNS, s.DNSDomains, s.DefaultGateway); err != nil {
			logf("dns: %v (skipped)", err)
		} else {
			undos = append(undos, func() { scutil("remove " + key + "\n") })
		}
	}
	return undo, nil
}

// currentGateway は dst への現在の経路の次ホップを route add 用の引数で返す。
// 直結のサブネット上なら nil。
func currentGateway(dst netip.Addr) ([]string, error) {
	out, err := exec.Command("route", "-n", "get", dst.String()).Output()
	if err != nil {
		return nil, fmt.Errorf("route get %s: %w", dst, err)
	}
	var gw, ifname string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		if !ok {
			continue
		}
		switch k {
		case "gateway":
			gw = strings.TrimSpace(v)
		case "interface":
			ifname = strings.TrimSpace(v)
		}
	}
	switch {
	case gw != "" && !strings.Contains(gw, ":") && net.ParseIP(gw) != nil:
		return []string{gw}, nil
	case ifname != "":
		return nil, nil // gateway が無い・MAC アドレス (link#N) = 直結
	}
	return nil, fmt.Errorf("no route to %s", dst)
}

// setDNS は SystemConfiguration の動的ストアに DNS 設定を登録する。
// 全ドメイン ("") か指定ドメインだけをこのサーバで解決する (supplemental resolver)。
func setDNS(key string, servers []netip.Addr, domains []string, all bool) error {
	match := domains
	if all {
		match = []string{""}
	}
	if len(match) == 0 {
		return errors.New("no search domain from DHCP; set nic.dns_domains or default_gateway")
	}
	var b strings.Builder
	b.WriteString("d.init\n")
	b.WriteString("d.add ServerAddresses *")
	for _, s := range servers {
		b.WriteString(" " + s.String())
	}
	b.WriteString("\n")
	b.WriteString("d.add SupplementalMatchDomains *")
	for _, d := range match {
		if d == "" {
			b.WriteString(` ""`)
		} else {
			b.WriteString(" " + d)
		}
	}
	b.WriteString("\n")
	if len(domains) > 0 {
		b.WriteString("d.add SearchDomains * " + strings.Join(domains, " ") + "\n")
	}
	b.WriteString("set " + key + "\n")
	return scutil(b.String())
}

func scutil(script string) error {
	cmd := exec.Command("scutil")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scutil: %v: %s", err, out)
	}
	return nil
}

const dnsKeyPrefix = "State:/Network/Service/secon-"

// CleanupStale は前回の異常終了で残った DNS 設定を消す。デーモン起動時に呼ぶ。
// (default_gateway 時の DNS 設定が残ると、すべての名前解決が届かない VPN 側 DNS に向いたままになる)
func CleanupStale(logf func(string, ...any)) {
	cmd := exec.Command("scutil")
	cmd.Stdin = strings.NewReader("list State:/Network/Service/secon-.*\n")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		// 形式: "  subKey [0] = State:/Network/Service/secon-utun4/DNS"
		_, key, ok := strings.Cut(sc.Text(), "= ")
		if !ok || !strings.HasPrefix(key, dnsKeyPrefix) {
			continue
		}
		if err := scutil("remove " + key + "\n"); err == nil {
			logf("removed stale DNS setting %s", key)
		}
	}
}
