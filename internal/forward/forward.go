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
// 片方向が EOF で終わったら相手に FIN だけ送り (half-close)、エラーで終わったら両方閉じる
// (相手が half-close を無視して黙ったままだと、もう片方向がいつまでも終わらないため)。
func Pipe(a, b net.Conn) {
	PipeReader(a, a, b)
}

// PipeReader は a からの読み込みに ra を使う Pipe (a の手前でバッファした分を含めて送るため)。
func PipeReader(a net.Conn, ra io.Reader, b net.Conn) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.Copy(b, ra); err != nil {
			a.Close()
			b.Close()
			return
		}
		closeWrite(b)
	}()
	if _, err := io.Copy(a, b); err != nil {
		a.Close()
		b.Close()
	} else {
		closeWrite(a)
	}
	<-done
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	} else {
		c.Close()
	}
}
