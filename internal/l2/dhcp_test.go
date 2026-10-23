package l2

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func TestParseClasslessRoutes(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []string // "<prefix> via <gateway>"
	}{
		{"single", []byte{8, 10, 10, 1, 16, 1}, []string{"10.0.0.0/8 via 10.1.16.1"}},
		{"default", []byte{0, 10, 1, 16, 1}, []string{"0.0.0.0/0 via 10.1.16.1"}},
		{"multiple", []byte{8, 10, 16, 192, 168, 10, 1, 16, 1}, []string{
			"10.0.0.0/8 via 10.1.16.1",
			"192.168.0.0/16 via 10.1.16.1",
		}},
		{"too short", []byte{8, 10}, nil},
		{"zero router", []byte{8, 10, 0, 0, 0, 0}, nil},
		{"width out of range", []byte{33, 10, 10, 1, 16, 1}, nil},
		{"truncated destination", []byte{16, 192, 10, 1, 16, 1}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routes, _ := parseClasslessRoutes(tc.in)
			if len(routes) != len(tc.want) {
				t.Fatalf("got %d routes, want %d (%v)", len(routes), len(tc.want), routes)
			}
			for i, want := range tc.want {
				got := routes[i].Prefix.String() + " via " + routes[i].Gateway.String()
				if got != want {
					t.Errorf("route %d = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestLeaseFromAckPrefersClasslessRoutes(t *testing.T) {
	ack, err := dhcpv4.New(
		dhcpv4.WithHwAddr(net.HardwareAddr{0x5e, 1, 2, 3, 4, 5}),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeAck),
		dhcpv4.WithOption(dhcpv4.OptGeneric(optClasslessStaticRoute, []byte{8, 10, 10, 1, 16, 1})),
		// Option 3 must be ignored when option 121 is present (RFC 3442).
		dhcpv4.WithOption(dhcpv4.OptRouter(net.IP{192, 168, 1, 1})),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(net.CIDRMask(22, 32))),
	)
	if err != nil {
		t.Fatal(err)
	}
	ack.YourIPAddr = net.IP{10, 1, 16, 50}.To4()

	l, err := leaseFromAck(ack)
	if err != nil {
		t.Fatal(err)
	}
	if l.Router.String() != "10.1.16.1" {
		t.Errorf("Router = %s, want 10.1.16.1", l.Router)
	}
	if len(l.Routes) != 1 || l.Routes[0].Prefix.String() != "10.0.0.0/8" {
		t.Errorf("Routes = %v, want [10.0.0.0/8]", l.Routes)
	}
}

func TestLeaseFromAckRouterFallback(t *testing.T) {
	ack, err := dhcpv4.New(
		dhcpv4.WithHwAddr(net.HardwareAddr{0x5e, 1, 2, 3, 4, 5}),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeAck),
		dhcpv4.WithOption(dhcpv4.OptRouter(net.IP{192, 168, 1, 1})),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(net.CIDRMask(24, 32))),
	)
	if err != nil {
		t.Fatal(err)
	}
	ack.YourIPAddr = net.IP{192, 168, 1, 50}.To4()

	l, err := leaseFromAck(ack)
	if err != nil {
		t.Fatal(err)
	}
	if l.Router.String() != "192.168.1.1" {
		t.Errorf("Router = %s, want 192.168.1.1", l.Router)
	}
	if len(l.Routes) != 0 {
		t.Errorf("Routes = %v, want none", l.Routes)
	}
}
