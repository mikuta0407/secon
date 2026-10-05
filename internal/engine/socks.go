package engine

import (
	"context"
	"os"

	"github.com/mikuta0407/secon/internal/l2"
	"github.com/mikuta0407/secon/internal/proto"
	"github.com/mikuta0407/secon/internal/usernet"
)

// runSocksMode はセッションをユーザ空間 TCP/IP スタックで終端する。
func runSocksMode(ctx context.Context, r *runner, sess *proto.Session) error {
	hostname, _ := os.Hostname()
	n, err := usernet.Start(ctx, sess, usernet.Config{
		MAC: r.mac, Hostname: hostname, Logf: r.logf, Static: staticLease(r.profile.Static),
		// DHCP で再取得してアドレスが変わったら状態表示にも反映する
		OnLease: func(l *l2.Lease) {
			r.update(func(s *Status) {
				if s.State == StateConnected {
					s.Address, s.Gateway, s.DNS = l.IP.String(), addrString(l.Router), addrStrings(l.DNS)
				}
			})
		},
	})
	if err != nil {
		return err
	}
	defer n.Close()

	r.dialer.Store(&dialerBox{n})
	defer r.dialer.Store(nil)

	l := n.Lease()
	r.connected(sess, "", l.IP.String(), addrString(l.Router), addrStrings(l.DNS))

	select {
	case <-ctx.Done():
		return nil
	case <-n.Done():
		return n.Err()
	}
}
