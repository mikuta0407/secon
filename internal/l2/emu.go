package l2

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	maxPendingPerIP = 16
	arpRetry        = time.Second
	arpRefresh      = 10 * time.Minute
)

// Emulator は L3 の仮想 NIC (macOS の utun) を L2 の VPN につなぐための ARP / Ethernet 処理。
// IPv4 のユニキャスト・ブロードキャスト・マルチキャストを扱う (IPv6 は対象外)。
type Emulator struct {
	mac net.HardwareAddr
	out chan []byte // VPN へ送るフレーム (ARP 応答・要求、解決待ちだったパケット)

	mu      sync.Mutex
	addr    netip.Prefix
	gw      netip.Addr
	arp     map[netip.Addr]arpEntry
	pending map[netip.Addr][][]byte
	asked   map[netip.Addr]time.Time
}

type arpEntry struct {
	mac  net.HardwareAddr
	seen time.Time
}

func NewEmulator(mac net.HardwareAddr) *Emulator {
	return &Emulator{
		mac:     mac,
		out:     make(chan []byte, 256),
		arp:     map[netip.Addr]arpEntry{},
		pending: map[netip.Addr][][]byte{},
		asked:   map[netip.Addr]time.Time{},
	}
}

// SetIPv4 は自分のアドレスとゲートウェイを設定する (DHCP 取得後)。
func (e *Emulator) SetIPv4(addr netip.Prefix, gw netip.Addr) {
	e.mu.Lock()
	e.addr, e.gw = addr, gw
	e.mu.Unlock()
}

// Out は VPN へ送るべきフレームのチャネル。
func (e *Emulator) Out() <-chan []byte { return e.out }

func (e *Emulator) enqueue(f []byte) {
	select {
	case e.out <- f:
	default: // 溢れたら捨てる (TCP 等が再送する)
	}
}

// FromVPN は VPN から来たフレームを処理し、OS に渡す IPv4 パケットを返す (無ければ nil)。
func (e *Emulator) FromVPN(f []byte) []byte {
	if len(f) < ethHeaderLen {
		return nil
	}
	dst := net.HardwareAddr(f[0:6])
	if !bytes.Equal(dst, e.mac) && dst[0]&1 == 0 { // 他人宛てユニキャストは無視
		return nil
	}
	switch binary.BigEndian.Uint16(f[12:14]) {
	case EtherTypeARP:
		e.handleARP(f[ethHeaderLen:])
		return nil
	case EtherTypeIPv4:
		return f[ethHeaderLen:]
	}
	return nil
}

func (e *Emulator) handleARP(a []byte) {
	if len(a) < 28 || binary.BigEndian.Uint16(a[0:2]) != 1 || binary.BigEndian.Uint16(a[2:4]) != EtherTypeIPv4 || a[4] != 6 || a[5] != 4 {
		return
	}
	op := binary.BigEndian.Uint16(a[6:8])
	sha := net.HardwareAddr(append([]byte(nil), a[8:14]...))
	spa := netip.AddrFrom4([4]byte(a[14:18]))
	tpa := netip.AddrFrom4([4]byte(a[24:28]))

	e.mu.Lock()
	my := e.addr.Addr()
	var flush [][]byte
	if spa.IsValid() && !spa.IsUnspecified() {
		// 要求・応答・Gratuitous ARP のいずれからも学習する
		e.arp[spa] = arpEntry{mac: sha, seen: time.Now()}
		if q := e.pending[spa]; len(q) > 0 {
			for _, pkt := range q {
				flush = append(flush, e.frame(sha, pkt))
			}
			delete(e.pending, spa)
			delete(e.asked, spa)
		}
	}
	e.mu.Unlock()

	for _, f := range flush {
		e.enqueue(f)
	}
	if op == 1 && my.IsValid() && tpa == my {
		e.enqueue(arpPacket(2, e.mac, my, sha, spa))
	}
}

// ToVPN は OS から来た IPv4 パケットを Ethernet フレームにする。
// 宛先 MAC が未解決なら nil を返し、ARP 要求を出してパケットを保留する。
func (e *Emulator) ToVPN(pkt []byte) []byte {
	if len(pkt) < ipv4HeaderLen || pkt[0]>>4 != 4 {
		return nil
	}
	dst := netip.AddrFrom4([4]byte(pkt[16:20]))
	pkt = append([]byte(nil), pkt...)

	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.addr.IsValid() {
		return nil
	}
	switch {
	case dst == netip.AddrFrom4([4]byte{255, 255, 255, 255}) || dst == broadcastOf(e.addr):
		return e.frame(BroadcastMAC, pkt)
	case dst.IsMulticast():
		d := dst.As4()
		return e.frame(net.HardwareAddr{0x01, 0x00, 0x5e, d[1] & 0x7f, d[2], d[3]}, pkt)
	}

	hop := dst
	if !e.addr.Contains(dst) {
		if !e.gw.IsValid() {
			return nil
		}
		hop = e.gw
	}
	now := time.Now()
	if ent, ok := e.arp[hop]; ok {
		if now.Sub(ent.seen) > arpRefresh && now.Sub(e.asked[hop]) > arpRetry {
			// 古いエントリは使い続けつつ裏で更新する
			e.asked[hop] = now
			e.enqueue(arpPacket(1, e.mac, e.addr.Addr(), nil, hop))
		}
		return e.frame(ent.mac, pkt)
	}
	if q := e.pending[hop]; len(q) < maxPendingPerIP {
		e.pending[hop] = append(q, pkt)
	}
	if now.Sub(e.asked[hop]) > arpRetry {
		e.asked[hop] = now
		e.enqueue(arpPacket(1, e.mac, e.addr.Addr(), nil, hop))
	}
	return nil
}

func (e *Emulator) frame(dst net.HardwareAddr, pkt []byte) []byte {
	f := make([]byte, ethHeaderLen+len(pkt))
	copy(f[0:6], dst)
	copy(f[6:12], e.mac)
	binary.BigEndian.PutUint16(f[12:14], EtherTypeIPv4)
	copy(f[ethHeaderLen:], pkt)
	return f
}

// arpPacket は ARP フレームを作る。op=1 (要求) なら tha は無視してブロードキャストする。
func arpPacket(op uint16, sha net.HardwareAddr, spa netip.Addr, tha net.HardwareAddr, tpa netip.Addr) []byte {
	dst := tha
	if op == 1 {
		dst, tha = BroadcastMAC, make(net.HardwareAddr, 6)
	}
	f := make([]byte, 0, 42)
	f = append(f, dst...)
	f = append(f, sha...)
	f = binary.BigEndian.AppendUint16(f, EtherTypeARP)
	f = append(f, 0, 1, 0x08, 0x00, 6, 4)
	f = binary.BigEndian.AppendUint16(f, op)
	s, t := spa.As4(), tpa.As4()
	f = append(f, sha...)
	f = append(f, s[:]...)
	f = append(f, tha...)
	return append(f, t[:]...)
}

func broadcastOf(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr().As4()
	host := uint32(1)<<(32-p.Bits()) - 1
	v := binary.BigEndian.Uint32(a[:]) | host
	binary.BigEndian.PutUint32(a[:], v)
	return netip.AddrFrom4(a)
}
