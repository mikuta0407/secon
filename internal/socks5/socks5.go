// Package socks5 は VPN 越しに接続する SOCKS5 サーバ (RFC 1928 / 1929)。
// 現状 CONNECT のみ対応。
package socks5

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mikuta0407/secon/internal/forward"
)

// Dialer は宛先への接続手段 (VPN 側)。
type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// Server は SOCKS5 サーバ。
type Server struct {
	Dialer   Dialer
	Username string // 空なら認証なし
	Password string
	Logf     func(format string, args ...any)
}

const (
	ver5 = 0x05

	methodNoAuth   = 0x00
	methodUserPass = 0x02
	methodNone     = 0xff

	cmdConnect = 0x01

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSuccess          = 0x00
	repGeneralFailure   = 0x01
	repNetUnreachable   = 0x03
	repHostUnreachable  = 0x04
	repConnRefused      = 0x05
	repCmdNotSupported  = 0x07
	repAddrNotSupported = 0x08
)

// Serve は ln で接続を受け付ける。ln が閉じられると戻る。
func (s *Server) Serve(ln net.Listener) error {
	if s.Logf == nil {
		s.Logf = log.Printf
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
			if err := s.handle(c); err != nil {
				s.Logf("socks5 %s: %v", c.RemoteAddr(), err)
			}
		}()
	}
}

func (s *Server) handle(c net.Conn) error {
	c.SetDeadline(time.Now().Add(30 * time.Second))
	r := bufio.NewReader(c)

	if err := s.negotiate(r, c); err != nil {
		return err
	}

	// リクエスト: VER CMD RSV ATYP DST.ADDR DST.PORT
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	if hdr[0] != ver5 {
		return fmt.Errorf("bad version %d", hdr[0])
	}
	addr, err := readAddr(r, hdr[3])
	if err != nil {
		reply(c, repAddrNotSupported, nil)
		return err
	}
	if hdr[1] != cmdConnect {
		reply(c, repCmdNotSupported, nil)
		return fmt.Errorf("unsupported command %d", hdr[1])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	dst, err := s.Dialer.DialContext(ctx, "tcp", addr)
	cancel()
	if err != nil {
		reply(c, replyCode(err), nil)
		return fmt.Errorf("connect %s: %w", addr, err)
	}
	defer dst.Close()
	if err := reply(c, repSuccess, dst.LocalAddr()); err != nil {
		return err
	}
	c.SetDeadline(time.Time{})

	// クライアントが既に送ってきたデータ (r にバッファ済み) も含めて中継する
	forward.PipeReader(c, r, dst)
	return nil
}

func (s *Server) negotiate(r *bufio.Reader, w io.Writer) error {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return err
	}
	if h[0] != ver5 {
		return fmt.Errorf("bad version %d", h[0])
	}
	methods := make([]byte, h[1])
	if _, err := io.ReadFull(r, methods); err != nil {
		return err
	}
	want := byte(methodNoAuth)
	if s.Username != "" {
		want = methodUserPass
	}
	for _, m := range methods {
		if m == want {
			if _, err := w.Write([]byte{ver5, want}); err != nil {
				return err
			}
			if want == methodUserPass {
				return s.authUserPass(r, w)
			}
			return nil
		}
	}
	w.Write([]byte{ver5, methodNone})
	return errors.New("no acceptable auth method")
}

// authUserPass は RFC 1929 のユーザ名/パスワード認証。
func (s *Server) authUserPass(r *bufio.Reader, w io.Writer) error {
	ver, err := r.ReadByte()
	if err != nil {
		return err
	}
	if ver != 0x01 {
		return fmt.Errorf("bad auth version %d", ver)
	}
	user, err := readLenString(r)
	if err != nil {
		return err
	}
	pass, err := readLenString(r)
	if err != nil {
		return err
	}
	ok := subtle.ConstantTimeCompare([]byte(user), []byte(s.Username)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(s.Password)) == 1
	if !ok {
		w.Write([]byte{0x01, 0x01})
		return errors.New("authentication failed")
	}
	_, err = w.Write([]byte{0x01, 0x00})
	return err
}

func readLenString(r *bufio.Reader) (string, error) {
	n, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	b := make([]byte, n)
	_, err = io.ReadFull(r, b)
	return string(b), err
}

func readAddr(r *bufio.Reader, atyp byte) (string, error) {
	var host string
	switch atyp {
	case atypIPv4:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		host = netip.AddrFrom4(b).String()
	case atypIPv6:
		var b [16]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		host = netip.AddrFrom16(b).String()
	case atypDomain:
		s, err := readLenString(r)
		if err != nil {
			return "", err
		}
		host = s
	default:
		return "", fmt.Errorf("unknown address type %d", atyp)
	}
	var p [2]byte
	if _, err := io.ReadFull(r, p[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(p[:])))), nil
}

func reply(w io.Writer, rep byte, bound net.Addr) error {
	b := []byte{ver5, rep, 0x00}
	ap, err := netip.ParseAddrPort(fmt.Sprint(bound))
	if bound == nil || err != nil {
		ap = netip.AddrPortFrom(netip.IPv4Unspecified(), 0)
	}
	if a := ap.Addr().Unmap(); a.Is4() {
		b = append(b, atypIPv4)
		b = append(b, a.AsSlice()...)
	} else {
		b = append(b, atypIPv6)
		b = append(b, a.AsSlice()...)
	}
	b = binary.BigEndian.AppendUint16(b, ap.Port())
	_, err = w.Write(b)
	return err
}

func replyCode(err error) byte {
	var ne net.Error
	switch {
	case errors.As(err, &ne) && ne.Timeout():
		return repHostUnreachable
	case errors.Is(err, context.DeadlineExceeded):
		return repHostUnreachable
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "refused"):
		return repConnRefused
	case strings.Contains(msg, "no route"), strings.Contains(msg, "unreachable"):
		return repNetUnreachable
	case strings.Contains(msg, "no such host"):
		return repHostUnreachable
	}
	return repGeneralFailure
}
