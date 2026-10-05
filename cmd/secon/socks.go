package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mikuta0407/secon/internal/forward"
	"github.com/mikuta0407/secon/internal/socks5"
	"github.com/mikuta0407/secon/internal/usernet"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// runSocks は SOCKS モードをフォアグラウンドで動かす (デーモン実装までの確認用)。
func runSocks(args []string) error {
	fs := flag.NewFlagSet("debug socks", flag.ExitOnError)
	cf := addConnFlags(fs)
	listen := fs.String("listen", "127.0.0.1:1080", "SOCKS5 の待ち受けアドレス (空なら無効)")
	var fwds multiFlag
	fs.Var(&fwds, "forward", "ポートフォワード listen=target (複数可)")
	fs.Parse(args)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sess, err := cf.connect(ctx)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	n, err := usernet.Start(ctx, sess, usernet.Config{MAC: randomMAC(), Hostname: hostname})
	if err != nil {
		return err
	}
	defer n.Close()

	var lns []net.Listener
	defer func() {
		for _, ln := range lns {
			ln.Close()
		}
	}()
	if *listen != "" {
		ln, err := net.Listen("tcp", *listen)
		if err != nil {
			return err
		}
		lns = append(lns, ln)
		log.Printf("socks5 listening on %s", ln.Addr())
		go (&socks5.Server{Dialer: n}).Serve(ln)
	}
	for _, f := range fwds {
		l, t, ok := strings.Cut(f, "=")
		if !ok {
			return fmt.Errorf("bad -forward %q", f)
		}
		ln, err := net.Listen("tcp", l)
		if err != nil {
			return err
		}
		lns = append(lns, ln)
		log.Printf("forward %s -> %s", ln.Addr(), t)
		go forward.Serve(ln, n, t, nil)
	}

	select {
	case <-ctx.Done():
		log.Print("shutting down")
		return nil
	case <-n.Done():
		return n.Err()
	}
}
