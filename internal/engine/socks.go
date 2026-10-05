package engine

import (
	"context"
	"os"

	"github.com/mikuta0407/secon/internal/proto"
	"github.com/mikuta0407/secon/internal/usernet"
)

// runSocksMode はセッションをユーザ空間 TCP/IP スタックで終端する。
func runSocksMode(ctx context.Context, r *runner, sess *proto.Session) error {
	hostname, _ := os.Hostname()
	n, err := usernet.Start(ctx, sess, usernet.Config{MAC: r.mac, Hostname: hostname, Logf: r.logf, Static: staticLease(r.profile.Static)})
	if err != nil {
		return err
	}
	defer n.Close()

	r.dialer.Store(&dialerBox{n})
	defer r.dialer.Store(nil)

	l := n.Lease()
	r.connected(sess, "", l.IP.String(), l.Router.String(), addrStrings(l.DNS))

	select {
	case <-ctx.Done():
		return nil
	case <-n.Done():
		return n.Err()
	}
}
