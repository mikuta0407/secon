package l2

import (
	"bytes"
	"net"
	"net/netip"
	"testing"
)

func TestUDPRoundTrip(t *testing.T) {
	in := UDPPacket{
		SrcMAC: net.HardwareAddr{0x5e, 1, 2, 3, 4, 5}, DstMAC: BroadcastMAC,
		SrcIP: netip.MustParseAddr("10.0.0.2"), DstIP: netip.MustParseAddr("10.0.0.1"),
		SrcPort: 68, DstPort: 67, Payload: []byte("hello"),
	}
	f := BuildUDP(in)
	out, ok := ParseUDP(f)
	if !ok {
		t.Fatal("parse failed")
	}
	if out.SrcIP != in.SrcIP || out.DstIP != in.DstIP || out.SrcPort != 68 || out.DstPort != 67 || !bytes.Equal(out.Payload, in.Payload) {
		t.Errorf("mismatch: %+v", out)
	}
	// IP ヘッダチェックサムの検証 (正しければ合計が 0xffff)
	if cs := checksum(f[14:34], 0); cs != 0xffff {
		t.Errorf("ip checksum invalid: %x", cs)
	}
}
