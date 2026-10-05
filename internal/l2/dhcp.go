package l2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// Lease は DHCP で得た設定。
type Lease struct {
	IP        netip.Prefix
	Router    netip.Addr
	DNS       []netip.Addr
	Domain    string     // ドメイン名 (option 15)
	Server    netip.Addr // DHCP サーバ ID
	MTU       int        // 0 なら指定なし
	LeaseTime time.Duration
	Acquired  time.Time
}

// RenewAt は T1 (リース時間の 1/2) の時刻。
func (l *Lease) RenewAt() time.Time { return l.Acquired.Add(l.LeaseTime / 2) }

// Expires はリースの期限。
func (l *Lease) Expires() time.Time { return l.Acquired.Add(l.LeaseTime) }

// Renewable は更新が必要か (リース時間 0 = 固定アドレスや無期限なら更新しない)。
func (l *Lease) Renewable() bool { return l.LeaseTime > 0 }

// DHCPClient は Ethernet フレームレベルで動く DHCPv4 クライアント。
// 受信フレームは HandleFrame に渡し、送信は send 関数で行う。
type DHCPClient struct {
	mac      net.HardwareAddr
	hostname string
	send     func([]byte) error

	mu      sync.Mutex
	xid     dhcpv4.TransactionID
	replies chan *dhcpv4.DHCPv4
}

func NewDHCPClient(mac net.HardwareAddr, hostname string, send func([]byte) error) *DHCPClient {
	return &DHCPClient{mac: mac, hostname: hostname, send: send, replies: make(chan *dhcpv4.DHCPv4, 8)}
}

// HandleFrame は DHCP 応答フレームなら取り込んで true を返す。
func (c *DHCPClient) HandleFrame(f []byte) bool {
	p, ok := ParseUDP(f)
	if !ok || p.DstPort != dhcpv4.ClientPort || p.SrcPort != dhcpv4.ServerPort {
		return false
	}
	if !bytes.Equal(p.DstMAC, c.mac) && !bytes.Equal(p.DstMAC, BroadcastMAC) {
		return false
	}
	m, err := dhcpv4.FromBytes(p.Payload)
	if err != nil || !bytes.Equal(m.ClientHWAddr, c.mac) {
		return true
	}
	c.mu.Lock()
	match := m.TransactionID == c.xid
	c.mu.Unlock()
	if match {
		select {
		case c.replies <- m:
		default:
		}
	}
	return true
}

func (c *DHCPClient) newXID() dhcpv4.TransactionID {
	xid, _ := dhcpv4.GenerateTransactionID()
	c.mu.Lock()
	c.xid = xid
	c.mu.Unlock()
	// 前のトランザクションの残りを捨てる
	for {
		select {
		case <-c.replies:
		default:
			return xid
		}
	}
}

func (c *DHCPClient) broadcast(m *dhcpv4.DHCPv4) error {
	return c.send(BuildUDP(UDPPacket{
		SrcMAC: c.mac, DstMAC: BroadcastMAC,
		SrcIP: netip.IPv4Unspecified(), DstIP: netip.AddrFrom4([4]byte{255, 255, 255, 255}),
		SrcPort: dhcpv4.ClientPort, DstPort: dhcpv4.ServerPort,
		Payload: m.ToBytes(),
	}))
}

// exchange は m を送り、want 型の応答を待つ (指数バックオフで再送)。
func (c *DHCPClient) exchange(ctx context.Context, m *dhcpv4.DHCPv4, want dhcpv4.MessageType) (*dhcpv4.DHCPv4, error) {
	wait := 2 * time.Second
	for {
		if err := c.broadcast(m); err != nil {
			return nil, err
		}
		timer := time.NewTimer(wait)
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			case r := <-c.replies:
				switch r.MessageType() {
				case want:
					timer.Stop()
					return r, nil
				case dhcpv4.MessageTypeNak:
					timer.Stop()
					return nil, errors.New("dhcp: NAK")
				}
				continue
			}
			break
		}
		wait = min(wait*2, 16*time.Second)
	}
}

func (c *DHCPClient) modifiers() []dhcpv4.Modifier {
	mods := []dhcpv4.Modifier{
		dhcpv4.WithHwAddr(c.mac),
		dhcpv4.WithBroadcast(true),
		dhcpv4.WithRequestedOptions(dhcpv4.OptionSubnetMask, dhcpv4.OptionRouter,
			dhcpv4.OptionDomainNameServer, dhcpv4.OptionInterfaceMTU, dhcpv4.OptionDomainName),
		dhcpv4.WithOption(dhcpv4.OptClientIdentifier(append([]byte{1}, c.mac...))),
	}
	if c.hostname != "" {
		mods = append(mods, dhcpv4.WithOption(dhcpv4.OptHostName(c.hostname)))
	}
	return mods
}

// Acquire は DISCOVER → OFFER → REQUEST → ACK でリースを取得する。
func (c *DHCPClient) Acquire(ctx context.Context) (*Lease, error) {
	disc, err := dhcpv4.NewDiscovery(c.mac, c.modifiers()...)
	if err != nil {
		return nil, err
	}
	disc.TransactionID = c.newXID()
	offer, err := c.exchange(ctx, disc, dhcpv4.MessageTypeOffer)
	if err != nil {
		return nil, fmt.Errorf("dhcp discover: %w", err)
	}
	req, err := dhcpv4.NewRequestFromOffer(offer, c.modifiers()...)
	if err != nil {
		return nil, err
	}
	ack, err := c.exchange(ctx, req, dhcpv4.MessageTypeAck)
	if err != nil {
		return nil, fmt.Errorf("dhcp request: %w", err)
	}
	return leaseFromAck(ack)
}

// Renew は現在のリースを延長する (ブロードキャストで REQUEST を送る)。
func (c *DHCPClient) Renew(ctx context.Context, l *Lease) (*Lease, error) {
	ip := l.IP.Addr().AsSlice()
	req, err := dhcpv4.New(append(c.modifiers(),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithClientIP(ip),
	)...)
	if err != nil {
		return nil, err
	}
	req.TransactionID = c.newXID()
	ack, err := c.exchange(ctx, req, dhcpv4.MessageTypeAck)
	if err != nil {
		return nil, fmt.Errorf("dhcp renew: %w", err)
	}
	return leaseFromAck(ack)
}

// Release はリースを返却する (応答は待たない)。
func (c *DHCPClient) Release(l *Lease) error {
	m, err := dhcpv4.New(
		dhcpv4.WithHwAddr(c.mac),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithClientIP(l.IP.Addr().AsSlice()),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(l.Server.AsSlice())),
	)
	if err != nil {
		return err
	}
	return c.broadcast(m)
}

func leaseFromAck(ack *dhcpv4.DHCPv4) (*Lease, error) {
	ip, ok := netip.AddrFromSlice(ack.YourIPAddr.To4())
	if !ok || ip.IsUnspecified() {
		return nil, errors.New("dhcp: ACK without address")
	}
	bits := 24
	if m := ack.SubnetMask(); m != nil {
		bits, _ = m.Size()
	}
	l := &Lease{IP: netip.PrefixFrom(ip, bits), Acquired: time.Now()}
	if rs := ack.Router(); len(rs) > 0 {
		l.Router, _ = netip.AddrFromSlice(rs[0].To4())
	}
	for _, d := range ack.DNS() {
		if a, ok := netip.AddrFromSlice(d.To4()); ok {
			l.DNS = append(l.DNS, a)
		}
	}
	if s := ack.ServerIdentifier(); s != nil {
		l.Server, _ = netip.AddrFromSlice(s.To4())
	}
	if mtu, err := dhcpv4.GetUint16(dhcpv4.OptionInterfaceMTU, ack.Options); err == nil {
		l.MTU = int(mtu)
	}
	l.Domain = ack.DomainName()
	l.LeaseTime = ack.IPAddressLeaseTime(time.Hour)
	return l, nil
}
