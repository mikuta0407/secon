// Package transport は VPN サーバまでの TCP / HTTP CONNECT / TLS 接続を扱う。
package transport

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Options は VPN サーバへの接続設定。
type Options struct {
	Server string   // host:port
	Proxy  *url.URL // http://[user:pass@]host:port。nil なら直結

	// サーバ証明書の検証。PinSHA256 (DER の SHA-256, hex) が空でなければ一致を要求する。
	// どちらも指定が無い場合は OS の信頼ストアで検証する。
	InsecureSkipVerify bool
	PinSHA256          string

	Timeout time.Duration // TCP 接続〜TLS ハンドシェイクまで
}

// ErrProxyAuth はプロキシ認証の失敗。
var ErrProxyAuth = errors.New("proxy authentication failed")

// Dial は TCP (必要ならプロキシ経由) → TLS まで行う。
func Dial(ctx context.Context, opts Options) (*tls.Conn, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	host, _, err := net.SplitHostPort(opts.Server)
	if err != nil {
		return nil, err
	}

	var d net.Dialer
	var raw net.Conn
	if opts.Proxy != nil {
		raw, err = d.DialContext(ctx, "tcp", opts.Proxy.Host)
		if err != nil {
			return nil, fmt.Errorf("connect to proxy: %w", err)
		}
		if err := httpConnect(ctx, raw, opts.Proxy, opts.Server); err != nil {
			raw.Close()
			return nil, err
		}
	} else {
		raw, err = d.DialContext(ctx, "tcp", opts.Server)
		if err != nil {
			return nil, err
		}
	}

	cfg := &tls.Config{ServerName: host}
	if opts.InsecureSkipVerify || opts.PinSHA256 != "" {
		// SoftEther サーバは自己署名証明書が普通なので、ピン留めは独自に検証する
		cfg.InsecureSkipVerify = true
		pin := strings.ToLower(strings.ReplaceAll(opts.PinSHA256, ":", ""))
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if pin == "" {
				return nil
			}
			if got := CertFingerprint(cs); got != pin {
				return fmt.Errorf("server certificate fingerprint mismatch: got %s", got)
			}
			return nil
		}
	}
	conn := tls.Client(raw, cfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	return conn, nil
}

// CertFingerprint はサーバ証明書 (リーフ) DER の SHA-256 を hex で返す。
func CertFingerprint(cs tls.ConnectionState) string {
	if len(cs.PeerCertificates) == 0 {
		return ""
	}
	sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
	return hex.EncodeToString(sum[:])
}

// httpConnect は HTTP プロキシに CONNECT を送ってトンネルを張る。
func httpConnect(ctx context.Context, conn net.Conn, proxy *url.URL, target string) error {
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
		defer conn.SetDeadline(time.Time{})
	}
	host, _, _ := net.SplitHostPort(target)
	var b strings.Builder
	fmt.Fprintf(&b, "CONNECT %s HTTP/1.0\r\n", target)
	fmt.Fprintf(&b, "Host: %s\r\n", host)
	b.WriteString("Content-Length: 0\r\nProxy-Connection: Keep-Alive\r\nPragma: no-cache\r\n")
	// 本家と同じく、ユーザ名とパスワードが両方あるときだけ認証ヘッダを送る
	if u := proxy.User; u != nil {
		if pass, ok := u.Password(); ok && u.Username() != "" && pass != "" {
			cred := base64.StdEncoding.EncodeToString([]byte(u.Username() + ":" + pass))
			fmt.Fprintf(&b, "Proxy-Authorization: Basic %s\r\n", cred)
		}
	}
	b.WriteString("\r\n")
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return err
	}

	// TLS 開始前にサーバからデータは来ないが、念のため 1 バイトずつ読んで先読みしない
	r := bufio.NewReaderSize(&byteReader{conn}, 16)
	status, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("proxy response: %w", err)
	}
	f := strings.Fields(status)
	if len(f) < 2 || !strings.HasPrefix(f[0], "HTTP/1.") {
		return fmt.Errorf("proxy: invalid response %q", strings.TrimSpace(status))
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("proxy response: %w", err)
		}
		if strings.TrimRight(line, "\r\n") == "" {
			break
		}
	}
	switch {
	case strings.HasPrefix(f[1], "2"):
		return nil
	case f[1] == "401" || f[1] == "403" || f[1] == "407":
		return ErrProxyAuth
	default:
		return fmt.Errorf("proxy: CONNECT failed: %s", strings.TrimSpace(status))
	}
}

// byteReader は 1 回の Read で 1 バイトだけ読む (bufio に先読みさせないため)。
type byteReader struct{ c net.Conn }

func (b *byteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return b.c.Read(p[:1])
}
