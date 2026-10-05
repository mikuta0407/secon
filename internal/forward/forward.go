// Package forward はローカルのポートで待ち受け、VPN 越しの宛先へ TCP を中継する。
package forward

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Dialer は宛先への接続手段。NIC モードでは net.Dialer、SOCKS モードでは usernet.Net。
type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// Rule は 1 つのポートフォワード設定。
type Rule struct {
	Listen string // 例 127.0.0.1:13389
	Target string // 例 10.0.0.5:3389
}

// Serve は ln への接続を rule.Target へ中継する。ln が閉じられると戻る。
func Serve(ln net.Listener, d Dialer, target string, logf func(string, ...any)) error {
	if logf == nil {
		logf = log.Printf
	}
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			dst, err := d.DialContext(ctx, "tcp", target)
			cancel()
			if err != nil {
				logf("forward %s -> %s: %v", ln.Addr(), target, err)
				return
			}
			defer dst.Close()
			Pipe(c, dst)
		}()
	}
}

// Pipe は a と b を双方向に中継し、両方向が終わるまで待つ。
func Pipe(a, b net.Conn) {
	done := make(chan struct{})
	go func() {
		io.Copy(b, a)
		closeWrite(b)
		close(done)
	}()
	io.Copy(a, b)
	closeWrite(a)
	<-done
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	} else {
		c.Close()
	}
}
