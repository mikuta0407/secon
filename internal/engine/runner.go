package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/forward"
	"github.com/mikuta0407/secon/internal/proto"
	"github.com/mikuta0407/secon/internal/socks5"
	"github.com/mikuta0407/secon/internal/transport"
)

const (
	minBackoff = time.Second
	maxBackoff = time.Minute
)

// runner は 1 プロファイルの接続ループ。
type runner struct {
	profile config.Profile
	m       *Manager
	mac     net.HardwareAddr // プロファイルごとに固定 (DHCP で同じアドレスをもらうため)

	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
	done   chan struct{}

	dialer atomic.Pointer[dialerBox]
}

type dialerBox struct{ d forward.Dialer }

func newRunner(p config.Profile, m *Manager) *runner {
	r := &runner{profile: p, m: m, mac: profileMAC(p.Name)}
	r.status = Status{Name: p.Name, Mode: p.Mode, Server: p.Server, State: StateDisconnected, Since: time.Now(), AutoConnect: p.AutoConnect}
	return r
}

// DialContext は現在のセッションで Dial する (未接続ならエラー)。リスナは再接続をまたいで生き続ける。
func (r *runner) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	b := r.dialer.Load()
	if b == nil {
		return nil, errors.New("vpn is not connected")
	}
	return b.d.DialContext(ctx, network, addr)
}

func (r *runner) getStatus() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.status
	s.DNS = append([]string(nil), s.DNS...)
	s.Forwards = append([]string(nil), s.Forwards...)
	return s
}

func (r *runner) update(f func(s *Status)) {
	r.mu.Lock()
	prev := r.status.State
	f(&r.status)
	if r.status.State != prev {
		r.status.Since = time.Now()
	}
	r.mu.Unlock()
	r.m.notifyChanged()
}

func (r *runner) setState(st State, err error) {
	r.update(func(s *Status) {
		s.State = st
		s.Error = ""
		if err != nil {
			s.Error = err.Error()
		}
		if st != StateConnected {
			s.Session, s.Address, s.Gateway, s.DNS = "", "", "", nil
		}
	})
}

func (r *runner) start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.done = cancel, make(chan struct{})
	go r.run(ctx, r.done)
}

func (r *runner) stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (r *runner) logf(format string, args ...any) {
	r.m.Logf("[%s] "+format, append([]any{r.profile.Name}, args...)...)
}

func (r *runner) run(ctx context.Context, done chan struct{}) {
	defer close(done)

	lns, err := r.listen()
	if err != nil {
		r.logf("%v", err)
		r.setState(StateFailed, err)
		r.mu.Lock()
		if r.done == done {
			r.cancel()
			r.cancel, r.done = nil, nil
		}
		r.mu.Unlock()
		return
	}
	defer func() {
		for _, ln := range lns {
			ln.Close()
		}
	}()

	backoff := minBackoff
	state := StateConnecting
	for {
		r.setState(state, nil)
		start := time.Now()
		err := r.session(ctx)
		if ctx.Err() != nil {
			break
		}
		r.logf("disconnected: %v", err)
		var se *proto.ServerError
		if errors.As(err, &se) && (se.Code == 8 || se.Code == 9 || se.Code == 7) {
			// HUB 無し・認証失敗は再試行しても直らないが、設定変更までゆっくり再試行する
			backoff = maxBackoff
		} else if time.Since(start) > maxBackoff {
			backoff = minBackoff
		}
		r.setState(StateReconnecting, err)
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		if ctx.Err() != nil {
			break
		}
		backoff = min(backoff*2, maxBackoff)
		state = StateReconnecting
	}
	r.setState(StateDisconnected, nil)
	r.logf("stopped")
}

// listen はプロファイルのリスナ (SOCKS5 / ポートフォワード) を開く。
func (r *runner) listen() ([]net.Listener, error) {
	p := r.profile
	var lns []net.Listener
	fail := func(err error) ([]net.Listener, error) {
		for _, ln := range lns {
			ln.Close()
		}
		return nil, err
	}
	var socksAddr string
	var fwds []string
	if p.Mode == config.ModeSocks && p.Socks.Listen != "" {
		ln, err := net.Listen("tcp", p.Socks.Listen)
		if err != nil {
			return fail(fmt.Errorf("socks5 listen: %w", err))
		}
		lns = append(lns, ln)
		socksAddr = ln.Addr().String()
		srv := &socks5.Server{Dialer: r, Username: p.Socks.Username, Password: p.Socks.Password, Logf: r.logf}
		go srv.Serve(ln)
	}
	for _, f := range p.Forwards {
		ln, err := net.Listen("tcp", f.Listen)
		if err != nil {
			return fail(fmt.Errorf("forward listen: %w", err))
		}
		lns = append(lns, ln)
		fwds = append(fwds, ln.Addr().String()+" -> "+f.Target)
		go forward.Serve(ln, r, f.Target, r.logf)
	}
	r.update(func(s *Status) { s.Socks, s.Forwards = socksAddr, fwds })
	return lns, nil
}

// session は 1 回分の接続を行い、切断されるまでブロックする。
func (r *runner) session(ctx context.Context) error {
	p := r.profile
	opts := transport.Options{
		Server:             p.Server,
		Proxy:              p.ProxyURL(),
		InsecureSkipVerify: p.Insecure,
		PinSHA256:          p.CertSHA256,
	}
	conn, err := transport.Dial(ctx, opts)
	if err != nil {
		return err
	}
	login := proto.LoginConfig{Hub: p.Hub, Username: p.User, Password: p.Password, AuthType: proto.AuthPassword}
	if p.Password == "" {
		login.AuthType = proto.AuthAnonymous
	}
	sess, _, err := proto.Handshake(conn, login)
	if err != nil {
		conn.Close()
		return err
	}
	defer sess.Close()
	r.logf("session %s established (%s)", sess.Welcome.SessionName, p.Server)
	if sess.Welcome.Message != "" {
		r.logf("server message: %s", sess.Welcome.Message)
	}

	mode, ok := modes[p.Mode]
	if !ok {
		return fmt.Errorf("mode %q is not supported on this platform", p.Mode)
	}
	return mode(ctx, r, sess)
}

// connected は接続完了をステータスに反映する。
func (r *runner) connected(sess *proto.Session, addr, gw string, dns []string) {
	r.update(func(s *Status) {
		s.State, s.Error = StateConnected, ""
		s.Session, s.Address, s.Gateway, s.DNS = sess.Welcome.SessionName, addr, gw, dns
	})
}

// modeFunc は接続済みセッションを各モードで動かし、終了するまでブロックする。
type modeFunc func(ctx context.Context, r *runner, sess *proto.Session) error

var modes = map[string]modeFunc{
	config.ModeSocks: runSocksMode,
}

// profileMAC はプロファイル名とホスト名から安定した MAC アドレスを作る。
func profileMAC(name string) net.HardwareAddr {
	host, _ := os.Hostname()
	h := proto.SHA0([]byte("secon-mac:" + host + ":" + name))
	mac := net.HardwareAddr(h[:6])
	mac[0] = 0x5e // ローカル管理・ユニキャスト
	return mac
}
