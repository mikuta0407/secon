// Package usernet は VPN セッションをユーザ空間の TCP/IP スタック (gVisor netstack) で終端する。
// OS に仮想 NIC を作らずに VPN の向こうへ Dial できる (SOCKS モード)。
package usernet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/link/ethernet"
	"gvisor.dev/gvisor/pkg/tcpip/network/arp"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"

	"github.com/mikuta0407/secon/internal/l2"
)

const (
	nicID      = 1
	defaultMTU = 1500
	maxFrame   = 1600
	txBatch    = 64
)

// FrameConn は Ethernet フレームを送受信する VPN セッション (proto.Session が満たす)。
type FrameConn interface {
	ReadFrame([]byte) (int, error)
	WriteFrames(...[]byte) error
	Close() error
}

// Config は usernet の設定。
type Config struct {
	MAC      net.HardwareAddr
	Hostname string // DHCP で送るホスト名
	Logf     func(format string, args ...any)
	// Static が nil でなければ DHCP を使わずにこのアドレスを使う
	Static *l2.Lease
}

// Net は VPN 上のユーザ空間ネットワーク。
type Net struct {
	conn FrameConn
	cfg  Config
	st   *stack.Stack
	ch   *channel.Endpoint
	dhcp *l2.DHCPClient

	lease    atomic.Pointer[l2.Lease]
	resolver *net.Resolver

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	errMu  sync.Mutex
	err    error
}

// Start は netstack を起動し、DHCP でアドレスを取得するまで待つ。
func Start(ctx context.Context, conn FrameConn, cfg Config) (*Net, error) {
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	n := &Net{conn: conn, cfg: cfg}
	n.ctx, n.cancel = context.WithCancel(context.Background())

	n.st = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, arp.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol4},
	})
	sack := tcpip.TCPSACKEnabled(true)
	n.st.SetTransportProtocolOption(tcp.ProtocolNumber, &sack)

	n.ch = channel.New(512, defaultMTU, tcpip.LinkAddress(cfg.MAC))
	if err := n.st.CreateNIC(nicID, ethernet.New(n.ch)); err != nil {
		return nil, fmt.Errorf("create nic: %s", err)
	}

	n.dhcp = l2.NewDHCPClient(cfg.MAC, cfg.Hostname, func(f []byte) error { return conn.WriteFrames(f) })

	n.wg.Add(2)
	go n.rxLoop()
	go n.txLoop()

	n.resolver = &net.Resolver{PreferGo: true, Dial: n.dialDNS}
	if cfg.Static != nil {
		if err := n.apply(cfg.Static); err != nil {
			n.Close()
			return nil, err
		}
		return n, nil
	}

	actx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	lease, err := n.dhcp.Acquire(actx)
	if err != nil {
		n.Close()
		return nil, err
	}
	if err := n.apply(lease); err != nil {
		n.Close()
		return nil, err
	}

	n.wg.Add(1)
	go n.renewLoop()
	return n, nil
}

// rxLoop は VPN から受けたフレームを DHCP クライアントか netstack に渡す。
func (n *Net) rxLoop() {
	defer n.wg.Done()
	buf := make([]byte, maxFrame)
	for {
		size, err := n.conn.ReadFrame(buf)
		if err != nil {
			n.fail(fmt.Errorf("vpn session: %w", err))
			return
		}
		f := buf[:size]
		if n.dhcp.HandleFrame(f) {
			continue
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(f)})
		n.ch.InjectInbound(0, pkt) // ethernet.Endpoint がヘッダから判別する
		pkt.DecRef()
	}
}

// txLoop は netstack が出したフレームを VPN へ送る。
func (n *Net) txLoop() {
	defer n.wg.Done()
	frames := make([][]byte, 0, txBatch)
	for {
		pkt := n.ch.ReadContext(n.ctx)
		if pkt == nil {
			return
		}
		frames = append(frames[:0], toBytes(pkt))
		for len(frames) < txBatch {
			p := n.ch.Read()
			if p == nil {
				break
			}
			frames = append(frames, toBytes(p))
		}
		if err := n.conn.WriteFrames(frames...); err != nil {
			n.fail(fmt.Errorf("vpn session: %w", err))
			return
		}
	}
}

func toBytes(pkt *stack.PacketBuffer) []byte {
	defer pkt.DecRef()
	b := pkt.ToBuffer()
	defer b.Release()
	return b.Flatten()
}

func (n *Net) renewLoop() {
	defer n.wg.Done()
	for {
		l := n.lease.Load()
		select {
		case <-n.ctx.Done():
			return
		case <-time.After(time.Until(l.RenewAt())):
		}
		ctx, cancel := context.WithTimeout(n.ctx, l.LeaseTime/4)
		nl, err := n.dhcp.Renew(ctx, l)
		cancel()
		if err != nil {
			if n.ctx.Err() != nil {
				return
			}
			n.cfg.Logf("dhcp renew failed: %v; reacquiring", err)
			if nl, err = n.dhcp.Acquire(n.ctx); err != nil {
				n.fail(err)
				return
			}
		}
		if err := n.apply(nl); err != nil {
			n.fail(err)
			return
		}
	}
}

// apply はリースを netstack に反映する。
func (n *Net) apply(l *l2.Lease) error {
	old := n.lease.Swap(l)
	if old != nil && old.IP != l.IP {
		n.st.RemoveAddress(nicID, tcpip.AddrFrom4(old.IP.Addr().As4()))
	}
	if old == nil || old.IP != l.IP {
		addr := tcpip.ProtocolAddress{
			Protocol: ipv4.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFrom4(l.IP.Addr().As4()),
				PrefixLen: l.IP.Bits(),
			},
		}
		if err := n.st.AddProtocolAddress(nicID, addr, stack.AddressProperties{}); err != nil {
			return fmt.Errorf("add address: %s", err)
		}
	}
	if l.MTU >= 576 && l.MTU <= defaultMTU {
		n.ch.SetMTU(uint32(l.MTU))
	}
	subnet, _ := tcpip.NewSubnet(tcpip.AddrFrom4(l.IP.Masked().Addr().As4()), tcpip.MaskFromBytes(net.CIDRMask(l.IP.Bits(), 32)))
	routes := []tcpip.Route{{Destination: subnet, NIC: nicID}}
	if l.Router.IsValid() {
		routes = append(routes, tcpip.Route{Destination: header.IPv4EmptySubnet, Gateway: tcpip.AddrFrom4(l.Router.As4()), NIC: nicID})
	}
	n.st.SetRouteTable(routes)
	if n.cfg.Static != nil {
		n.cfg.Logf("static: address %s router %s dns %v", l.IP, l.Router, l.DNS)
	} else {
		n.cfg.Logf("dhcp: address %s router %s dns %v lease %s", l.IP, l.Router, l.DNS, l.LeaseTime)
	}
	return nil
}

// Lease は現在の DHCP リース。
func (n *Net) Lease() *l2.Lease { return n.lease.Load() }

// Done はセッションが終了すると閉じる。
func (n *Net) Done() <-chan struct{} { return n.ctx.Done() }

// Err は終了理由。
func (n *Net) Err() error {
	n.errMu.Lock()
	defer n.errMu.Unlock()
	return n.err
}

func (n *Net) fail(err error) {
	n.errMu.Lock()
	if n.err == nil && n.ctx.Err() == nil {
		n.err = err
	}
	n.errMu.Unlock()
	n.cancel()
	n.conn.Close()
}

// Close は DHCP リースを返却して停止する。
func (n *Net) Close() error {
	if l := n.lease.Load(); l != nil && n.ctx.Err() == nil && n.cfg.Static == nil {
		n.dhcp.Release(l)
	}
	n.cancel()
	n.conn.Close()
	n.wg.Wait()
	n.st.Close()
	return nil
}

// DialContext は VPN 側へ TCP/UDP 接続する。ホスト名は VPN 側 DNS で解決する。
func (n *Net) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q", portStr)
	}
	ip, err := n.LookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	fa := tcpip.FullAddress{NIC: nicID, Addr: tcpip.AddrFrom4(ip.As4()), Port: uint16(port)}
	switch network {
	case "tcp", "tcp4":
		return gonet.DialContextTCP(ctx, n.st, fa, ipv4.ProtocolNumber)
	case "udp", "udp4":
		return gonet.DialUDP(n.st, nil, &fa, ipv4.ProtocolNumber)
	default:
		return nil, fmt.Errorf("unsupported network %q", network)
	}
}

// LookupIP はホスト名を VPN 側 DNS で IPv4 アドレスに解決する。
func (n *Net) LookupIP(ctx context.Context, host string) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !ip.Unmap().Is4() {
			return netip.Addr{}, errors.New("IPv6 is not supported")
		}
		return ip.Unmap(), nil
	}
	ips, err := n.resolver.LookupNetIP(ctx, "ip4", host)
	if err != nil {
		return netip.Addr{}, err
	}
	return ips[0], nil
}

// dialDNS は Go のリゾルバ用に、DHCP で得た DNS サーバへ netstack 経由で接続する。
func (n *Net) dialDNS(ctx context.Context, network, _ string) (net.Conn, error) {
	l := n.lease.Load()
	if l == nil || len(l.DNS) == 0 {
		return nil, errors.New("no DNS server from DHCP")
	}
	fa := tcpip.FullAddress{NIC: nicID, Addr: tcpip.AddrFrom4(l.DNS[0].As4()), Port: 53}
	if network == "tcp" || network == "tcp4" {
		return gonet.DialContextTCP(ctx, n.st, fa, ipv4.ProtocolNumber)
	}
	return gonet.DialUDP(n.st, nil, &fa, ipv4.ProtocolNumber)
}
