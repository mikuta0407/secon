package engine

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"time"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/l2"
	"github.com/mikuta0407/secon/internal/nic"
	"github.com/mikuta0407/secon/internal/proto"
)

func init() {
	modes[config.ModeNIC] = runNICMode
}

// runNICMode はセッションを OS の仮想 NIC につなぎ、アドレス・経路・DNS を設定する。
func runNICMode(ctx context.Context, r *runner, sess *proto.Session) error {
	p := r.profile
	dev, err := nic.Open(nic.InterfaceName(p.Name), r.mac, 1500)
	if err != nil {
		return err
	}
	defer dev.Close()

	hostname, _ := os.Hostname()
	dhcp := l2.NewDHCPClient(r.mac, hostname, func(f []byte) error { return sess.WriteFrames(f) })

	errc := make(chan error, 2)
	// VPN → NIC (自分宛ての DHCP 応答は横取りする)
	go func() {
		buf := make([]byte, proto.MaxFrameSize)
		for {
			n, err := sess.ReadFrame(buf)
			if err != nil {
				errc <- fmt.Errorf("vpn session: %w", err)
				return
			}
			if dhcp.HandleFrame(buf[:n]) {
				continue
			}
			if err := dev.WriteFrame(buf[:n]); err != nil {
				errc <- fmt.Errorf("nic write: %w", err)
				return
			}
		}
	}()
	// NIC → VPN
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := dev.ReadFrame(buf)
			if err != nil {
				errc <- fmt.Errorf("nic read: %w", err)
				return
			}
			if n > proto.MaxFrameSize {
				continue
			}
			if err := sess.WriteFrames(buf[:n]); err != nil {
				errc <- fmt.Errorf("vpn session: %w", err)
				return
			}
		}
	}()

	lease := staticLease(p.Static)
	var renew <-chan time.Time // 固定アドレスなら更新しない
	if lease == nil {
		actx, cancel := context.WithTimeout(ctx, 30*time.Second)
		l, err := dhcp.Acquire(actx)
		cancel()
		if err != nil {
			return err
		}
		lease = l
		defer func() { dhcp.Release(lease) }()
	}

	settings := nicSettings(p, lease, bypassAddrs(ctx, p, sess.RemoteAddr()))
	undo, err := nic.Apply(dev, settings, r.logf)
	if err != nil {
		return err
	}
	defer undo()

	r.dialer.Store(&dialerBox{&net.Dialer{}})
	defer r.dialer.Store(nil)
	r.connected(sess, dev.Name(), lease.IP.String(), addrString(lease.Router), addrStrings(lease.DNS))
	r.logf("nic %s: %s gw %s routes %v default_gateway=%v", dev.Name(), lease.IP, lease.Router, settings.Routes, settings.DefaultGateway)

	for {
		if lease.Renewable() {
			renew = time.After(time.Until(lease.RenewAt()))
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			return err
		case <-renew:
			// T1 からリース期限まで再送を続ける (一時的に DHCP サーバが応答しなくても切断しない)
			rctx, cancel := context.WithDeadline(ctx, lease.Expires())
			nl, err := dhcp.Renew(rctx, lease)
			cancel()
			if err != nil {
				return fmt.Errorf("dhcp renew: %w", err)
			}
			if nl.IP != lease.IP || nl.Router != lease.Router {
				// アドレスが変わったらセッションごと張り直す
				return fmt.Errorf("dhcp lease changed (%s -> %s)", lease.IP, nl.IP)
			}
			lease = nl
		}
	}
}

// staticLease は固定アドレス設定をリース形式にする (未設定なら nil)。
func staticLease(s config.Static) *l2.Lease {
	if !s.Enabled() {
		return nil
	}
	l := &l2.Lease{IP: netip.MustParsePrefix(s.Address), Acquired: time.Now()}
	if s.Gateway != "" {
		l.Router = netip.MustParseAddr(s.Gateway)
	}
	for _, d := range s.DNS {
		l.DNS = append(l.DNS, netip.MustParseAddr(d))
	}
	return l
}

// bypassAddrs は default_gateway 時に元の経路で送る宛先。接続先 (直結ならサーバ、プロキシ経由なら
// プロキシ) に加え、プロキシ経由なら VPN サーバ自体も含める (ローカルのプロキシが VPN 内を通らないように)。
func bypassAddrs(ctx context.Context, p config.Profile, remote net.Addr) []netip.Addr {
	if !p.NIC.DefaultGateway {
		return nil
	}
	var out []netip.Addr
	add := func(a netip.Addr) {
		a = a.Unmap()
		if a.Is4() && !a.IsLoopback() && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	add(nic.HostAddr(remote))
	if p.Proxy != "" {
		host, _, _ := net.SplitHostPort(p.Server)
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ips, _ := net.DefaultResolver.LookupNetIP(rctx, "ip4", host)
		cancel()
		for _, ip := range ips {
			add(ip)
		}
	}
	return out
}

func addrString(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}

func nicSettings(p config.Profile, l *l2.Lease, bypass []netip.Addr) nic.Settings {
	s := nic.Settings{
		Address:        l.IP,
		Gateway:        l.Router,
		MTU:            l.MTU,
		DefaultGateway: p.NIC.DefaultGateway,
		Bypass:         bypass,
	}
	for _, r := range p.NIC.Routes {
		s.Routes = append(s.Routes, netip.MustParsePrefix(r).Masked())
	}
	if p.NIC.DNS {
		s.DNS = l.DNS
		s.DNSDomains = p.NIC.DNSDomains
		if len(s.DNSDomains) == 0 && l.Domain != "" {
			s.DNSDomains = []string{l.Domain}
		}
	}
	return s
}

func addrStrings(as []netip.Addr) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.String())
	}
	return out
}
