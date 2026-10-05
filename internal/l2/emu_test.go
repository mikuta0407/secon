package l2

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
)

func ipPacket(src, dst string) []byte {
	p := make([]byte, 20)
	p[0] = 0x45
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	copy(p[12:16], s[:])
	copy(p[16:20], d[:])
	return p
}

func recv(t *testing.T, e *Emulator) []byte {
	t.Helper()
	select {
	case f := <-e.Out():
		return f
	default:
		t.Fatal("no frame queued")
		return nil
	}
}

func TestEmulatorResolveAndFlush(t *testing.T) {
	me := net.HardwareAddr{0x5e, 0, 0, 0, 0, 1}
	gwMAC := net.HardwareAddr{0x5e, 0, 0, 0, 0, 2}
	e := NewEmulator(me)
	e.SetIPv4(netip.MustParsePrefix("10.0.0.5/24"), netip.MustParseAddr("10.0.0.1"))

	// 外部宛て → ゲートウェイの ARP 要求が出てパケットは保留
	if f := e.ToVPN(ipPacket("10.0.0.5", "8.8.8.8")); f != nil {
		t.Fatal("expected pending")
	}
	req := recv(t, e)
	if binary.BigEndian.Uint16(req[12:14]) != EtherTypeARP || !bytes.Equal(req[38:42], []byte{10, 0, 0, 1}) {
		t.Fatalf("bad arp request % x", req)
	}

	// ゲートウェイの ARP 応答 → 保留パケットが送られる
	reply := arpPacket(2, gwMAC, netip.MustParseAddr("10.0.0.1"), me, netip.MustParseAddr("10.0.0.5"))
	if e.FromVPN(reply) != nil {
		t.Fatal("arp should not reach OS")
	}
	f := recv(t, e)
	if !bytes.Equal(f[0:6], gwMAC) || binary.BigEndian.Uint16(f[12:14]) != EtherTypeIPv4 {
		t.Fatalf("bad flushed frame % x", f[:14])
	}

	// 以降はすぐにフレーム化される (同一サブネットも ARP 済みなら直接)
	if f := e.ToVPN(ipPacket("10.0.0.5", "1.1.1.1")); f == nil || !bytes.Equal(f[0:6], gwMAC) {
		t.Fatal("expected immediate frame via gateway")
	}
	// ブロードキャスト
	if f := e.ToVPN(ipPacket("10.0.0.5", "10.0.0.255")); f == nil || !bytes.Equal(f[0:6], BroadcastMAC) {
		t.Fatal("expected broadcast frame")
	}
}

func TestEmulatorAnswersARP(t *testing.T) {
	me := net.HardwareAddr{0x5e, 0, 0, 0, 0, 1}
	peer := net.HardwareAddr{0x5e, 0, 0, 0, 0, 9}
	e := NewEmulator(me)
	e.SetIPv4(netip.MustParsePrefix("10.0.0.5/24"), netip.Addr{})

	e.FromVPN(arpPacket(1, peer, netip.MustParseAddr("10.0.0.9"), nil, netip.MustParseAddr("10.0.0.5")))
	r := recv(t, e)
	if !bytes.Equal(r[0:6], peer) || binary.BigEndian.Uint16(r[20:22]) != 2 || !bytes.Equal(r[22:28], me) {
		t.Fatalf("bad arp reply % x", r)
	}
	// 要求元を学習しているので、すぐ送れる
	if f := e.ToVPN(ipPacket("10.0.0.5", "10.0.0.9")); f == nil || !bytes.Equal(f[0:6], peer) {
		t.Fatal("expected learned entry")
	}
	// 他人宛てユニキャストの IP は OS に渡さない
	other := e.frame(net.HardwareAddr{0x5e, 1, 1, 1, 1, 1}, ipPacket("10.0.0.9", "10.0.0.7"))
	if e.FromVPN(other) != nil {
		t.Fatal("unicast to other host must be dropped")
	}
}
