// Package l2 は Ethernet フレーム上の小さな処理 (UDP の組み立て・解析、DHCP クライアント) を提供する。
// SOCKS モードの netstack と macOS の utun L2 エミュレーションで共有する。
package l2

import (
	"encoding/binary"
	"net"
	"net/netip"
)

const (
	EtherTypeIPv4 = 0x0800
	EtherTypeARP  = 0x0806
	EtherTypeIPv6 = 0x86dd

	ethHeaderLen  = 14
	ipv4HeaderLen = 20
	udpHeaderLen  = 8
)

var BroadcastMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// UDPPacket は Ethernet/IPv4/UDP フレームの要素。
type UDPPacket struct {
	SrcMAC, DstMAC   net.HardwareAddr
	SrcIP, DstIP     netip.Addr
	SrcPort, DstPort uint16
	Payload          []byte
}

// BuildUDP は Ethernet/IPv4/UDP フレームを組み立てる (UDP チェックサムは計算する)。
func BuildUDP(p UDPPacket) []byte {
	total := ipv4HeaderLen + udpHeaderLen + len(p.Payload)
	f := make([]byte, ethHeaderLen+total)
	copy(f[0:6], p.DstMAC)
	copy(f[6:12], p.SrcMAC)
	binary.BigEndian.PutUint16(f[12:14], EtherTypeIPv4)

	ip := f[ethHeaderLen:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(total))
	ip[8] = 64 // TTL
	ip[9] = 17 // UDP
	src, dst := p.SrcIP.As4(), p.DstIP.As4()
	copy(ip[12:16], src[:])
	copy(ip[16:20], dst[:])
	binary.BigEndian.PutUint16(ip[10:12], ^checksum(ip[:ipv4HeaderLen], 0))

	udp := ip[ipv4HeaderLen:]
	binary.BigEndian.PutUint16(udp[0:2], p.SrcPort)
	binary.BigEndian.PutUint16(udp[2:4], p.DstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpHeaderLen+len(p.Payload)))
	copy(udp[udpHeaderLen:], p.Payload)

	// 疑似ヘッダ込みのチェックサム
	var pseudo [12]byte
	copy(pseudo[0:4], src[:])
	copy(pseudo[4:8], dst[:])
	pseudo[9] = 17
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(udpHeaderLen+len(p.Payload)))
	cs := ^checksum(udp[:udpHeaderLen+len(p.Payload)], uint32(checksum(pseudo[:], 0)))
	if cs == 0 {
		cs = 0xffff
	}
	binary.BigEndian.PutUint16(udp[6:8], cs)
	return f
}

// ParseUDP は Ethernet/IPv4/UDP フレームを解析する。それ以外なら ok=false。
// Payload は f の部分スライス。
func ParseUDP(f []byte) (p UDPPacket, ok bool) {
	if len(f) < ethHeaderLen+ipv4HeaderLen+udpHeaderLen || binary.BigEndian.Uint16(f[12:14]) != EtherTypeIPv4 {
		return p, false
	}
	ip := f[ethHeaderLen:]
	ihl := int(ip[0]&0x0f) * 4
	if ip[0]>>4 != 4 || ihl < ipv4HeaderLen || ip[9] != 17 || len(ip) < ihl+udpHeaderLen {
		return p, false
	}
	// 断片化されたパケットは扱わない
	if binary.BigEndian.Uint16(ip[6:8])&0x3fff != 0 {
		return p, false
	}
	total := int(binary.BigEndian.Uint16(ip[2:4]))
	if total < ihl+udpHeaderLen || total > len(ip) {
		return p, false
	}
	udp := ip[ihl:total]
	ulen := int(binary.BigEndian.Uint16(udp[4:6]))
	if ulen < udpHeaderLen || ulen > len(udp) {
		return p, false
	}
	p.DstMAC = net.HardwareAddr(f[0:6])
	p.SrcMAC = net.HardwareAddr(f[6:12])
	p.SrcIP = netip.AddrFrom4([4]byte(ip[12:16]))
	p.DstIP = netip.AddrFrom4([4]byte(ip[16:20]))
	p.SrcPort = binary.BigEndian.Uint16(udp[0:2])
	p.DstPort = binary.BigEndian.Uint16(udp[2:4])
	p.Payload = udp[udpHeaderLen:ulen]
	return p, true
}

func checksum(b []byte, initial uint32) uint16 {
	sum := initial
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return uint16(sum)
}
