package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/mikuta0407/secon/internal/proto"
	"github.com/mikuta0407/secon/internal/transport"
)

func runDebug(args []string) error {
	if len(args) < 1 {
		usage()
	}
	switch args[0] {
	case "dump":
		return runDump(args[1:])
	case "socks":
		return runSocks(args[1:])
	}
	usage()
	return nil
}

// connFlags は接続に関する共通フラグ。
type connFlags struct {
	server, hub, user, pass, proxy *string
	insecure                       *bool
}

func addConnFlags(fs *flag.FlagSet) *connFlags {
	return &connFlags{
		server:   fs.String("server", "", "VPN サーバ host:port"),
		hub:      fs.String("hub", "VPN", "仮想 HUB 名"),
		user:     fs.String("user", "", "ユーザ名"),
		pass:     fs.String("pass", "", "パスワード (空なら匿名認証)"),
		proxy:    fs.String("proxy", "", "HTTP プロキシ URL"),
		insecure: fs.Bool("insecure", false, "サーバ証明書を検証しない"),
	}
}

func (cf *connFlags) connect(ctx context.Context) (*proto.Session, error) {
	opts := transport.Options{Server: *cf.server, InsecureSkipVerify: *cf.insecure}
	if *cf.proxy != "" {
		u, err := url.Parse(*cf.proxy)
		if err != nil {
			return nil, err
		}
		opts.Proxy = u
	}
	conn, err := transport.Dial(ctx, opts)
	if err != nil {
		return nil, err
	}
	log.Printf("TLS connected: %s (cert sha256=%s)", conn.RemoteAddr(), transport.CertFingerprint(conn.ConnectionState()))

	login := proto.LoginConfig{Hub: *cf.hub, Username: *cf.user, Password: *cf.pass, AuthType: proto.AuthPassword}
	if *cf.pass == "" {
		login.AuthType = proto.AuthAnonymous
	}
	sess, hello, err := proto.Handshake(conn, login)
	if err != nil {
		conn.Close()
		return nil, err
	}
	w := sess.Welcome
	log.Printf("server: %s ver=%d build=%d", hello.Server, hello.Version, hello.Build)
	log.Printf("session: %s / %s max_connection=%d timeout=%s", w.SessionName, w.ConnectionName, w.MaxConnection, w.Timeout)
	if w.Message != "" {
		log.Printf("server message: %s", w.Message)
	}
	return sess, nil
}

func runDump(args []string) error {
	fs := flag.NewFlagSet("debug dump", flag.ExitOnError)
	cf := addConnFlags(fs)
	pcapPath := fs.String("pcap", "", "受信フレームを書き出す pcap ファイル")
	dur := fs.Duration("duration", 10*time.Second, "受信する時間")
	arp := fs.String("arp", "", "送信確認用に ARP 要求を出す IPv4 アドレス")
	fs.Parse(args)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	sess, err := cf.connect(ctx)
	if err != nil {
		return err
	}
	defer sess.Close()

	var pw *pcapWriter
	if *pcapPath != "" {
		f, err := os.Create(*pcapPath)
		if err != nil {
			return err
		}
		defer f.Close()
		if pw, err = newPcapWriter(f); err != nil {
			return err
		}
	}

	mac := randomMAC()
	if *arp != "" {
		target, err := netip.ParseAddr(*arp)
		if err != nil || !target.Is4() {
			return fmt.Errorf("invalid -arp address %q", *arp)
		}
		if err := sess.WriteFrames(arpProbe(mac, target)); err != nil {
			return err
		}
		log.Printf("sent ARP probe for %s from %s", target, mac)
	}

	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(*dur):
		}
		sess.Close()
	}()

	buf := make([]byte, proto.MaxFrameSize)
	count := 0
	for {
		n, err := sess.ReadFrame(buf)
		if err != nil {
			if count > 0 && (errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF)) {
				break
			}
			if ctx.Err() == nil && errors.Is(err, net.ErrClosed) {
				break
			}
			return err
		}
		count++
		f := buf[:n]
		log.Printf("frame %4d: %s", n, describeFrame(f, mac))
		if pw != nil {
			pw.write(f)
		}
	}
	log.Printf("received %d frames", count)
	return nil
}

func randomMAC() net.HardwareAddr {
	m := make(net.HardwareAddr, 6)
	rand.Read(m)
	m[0] = 0x5e // SoftEther と同じくローカル管理アドレス
	return m
}

// arpProbe は送信元 IP 0.0.0.0 の ARP 要求 (RFC 5227 の probe) を作る。
func arpProbe(src net.HardwareAddr, target netip.Addr) []byte {
	f := make([]byte, 0, 42)
	f = append(f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)
	f = append(f, src...)
	f = append(f, 0x08, 0x06)             // ARP
	f = append(f, 0, 1, 8, 0, 6, 4, 0, 1) // Ethernet/IPv4, request
	f = append(f, src...)
	f = append(f, 0, 0, 0, 0)
	f = append(f, 0, 0, 0, 0, 0, 0)
	t := target.As4()
	return append(f, t[:]...)
}

func describeFrame(f []byte, self net.HardwareAddr) string {
	if len(f) < 14 {
		return "short frame"
	}
	dst, src := net.HardwareAddr(f[0:6]), net.HardwareAddr(f[6:12])
	et := binary.BigEndian.Uint16(f[12:14])
	s := fmt.Sprintf("%s -> %s type=%04x", src, dst, et)
	if et == 0x0806 && len(f) >= 42 {
		op := binary.BigEndian.Uint16(f[20:22])
		spa, tpa := netip.AddrFrom4([4]byte(f[28:32])), netip.AddrFrom4([4]byte(f[38:42]))
		s += fmt.Sprintf(" ARP op=%d %s(%s) -> %s", op, spa, net.HardwareAddr(f[22:28]), tpa)
		if op == 2 && dst.String() == self.String() {
			s += "  <== reply to us"
		}
	}
	return s
}

type pcapWriter struct{ w io.Writer }

func newPcapWriter(w io.Writer) (*pcapWriter, error) {
	h := make([]byte, 24)
	binary.LittleEndian.PutUint32(h[0:], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(h[4:], 2)
	binary.LittleEndian.PutUint16(h[6:], 4)
	binary.LittleEndian.PutUint32(h[16:], 65535)
	binary.LittleEndian.PutUint32(h[20:], 1) // Ethernet
	_, err := w.Write(h)
	return &pcapWriter{w}, err
}

func (p *pcapWriter) write(f []byte) {
	now := time.Now()
	h := make([]byte, 16)
	binary.LittleEndian.PutUint32(h[0:], uint32(now.Unix()))
	binary.LittleEndian.PutUint32(h[4:], uint32(now.Nanosecond()/1000))
	binary.LittleEndian.PutUint32(h[8:], uint32(len(f)))
	binary.LittleEndian.PutUint32(h[12:], uint32(len(f)))
	p.w.Write(h)
	p.w.Write(f)
}
