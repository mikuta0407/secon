package engine

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
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

	actx, cancel := context.WithTimeout(ctx, 30*time.Second)
	lease, err := dhcp.Acquire(actx)
	cancel()
	if err != nil {
		return err
	}
	defer dhcp.Release(lease)

	settings := nicSettings(p, lease, sess.RemoteAddr())
	undo, err := nic.Apply(dev, settings, r.logf)
	if err != nil {
		return err
	}
	defer undo()

	r.dialer.Store(&dialerBox{&net.Dialer{}})
	defer r.dialer.Store(nil)
	r.connected(sess, dev.Name(), lease.IP.String(), lease.Router.String(), addrStrings(lease.DNS))
	r.logf("nic %s: %s gw %s routes %v default_gateway=%v", dev.Name(), lease.IP, lease.Router, settings.Routes, settings.DefaultGateway)

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			return err
		case <-time.After(time.Until(lease.RenewAt())):
			rctx, cancel := context.WithTimeout(ctx, lease.LeaseTime/4)
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

func nicSettings(p config.Profile, l *l2.Lease, remote net.Addr) nic.Settings {
	s := nic.Settings{
		Address:        l.IP,
		Gateway:        l.Router,
		MTU:            l.MTU,
		DefaultGateway: p.NIC.DefaultGateway,
		Bypass:         nic.HostAddr(remote),
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
