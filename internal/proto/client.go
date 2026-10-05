package proto

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// 認証方式 (authtype)
const (
	AuthAnonymous = 0
	AuthPassword  = 1
)

const (
	clientStr   = "secon SoftEther Compatible Client"
	clientVer   = 502
	clientBuild = 5187

	connectingTimeout = 15 * time.Second
	maxNoop           = 30
)

// LoginConfig は認証情報。
type LoginConfig struct {
	Hub      string
	Username string
	Password string // AuthType=AuthPassword のとき
	AuthType uint32
	UniqueID []byte // 20 バイト。nil なら乱数
}

// Hello はサーバの Hello。
type Hello struct {
	Server  string
	Version uint32
	Build   uint32
	Random  []byte
}

// Welcome は認証成功時のサーバ応答。
type Welcome struct {
	SessionName    string
	ConnectionName string
	SessionKey     []byte
	MaxConnection  uint32
	UseEncrypt     bool
	UseCompress    bool
	Timeout        time.Duration
	Message        string
}

// ServerError はサーバが返したエラーコード。
type ServerError struct{ Code uint32 }

var errorNames = map[uint32]string{
	1: "connect failed", 2: "server is not VPN", 3: "disconnected", 4: "protocol error",
	7: "auth type not supported", 8: "hub not found", 9: "authentication failed",
	10: "hub stopping", 11: "session removed", 12: "access denied", 13: "session timeout",
	14: "invalid protocol", 15: "too many connection", 16: "hub is busy",
	20: "too many user session", 23: "internal error", 33: "not supported",
	63: "too many user", 105: "server can't accept", 109: "ip address denied",
}

func (e *ServerError) Error() string {
	if n, ok := errorNames[e.Code]; ok {
		return fmt.Sprintf("server error %d (%s)", e.Code, n)
	}
	return fmt.Sprintf("server error %d", e.Code)
}

// Handshake は TLS 接続上で signature → Hello → 認証 → Welcome を行い、
// データチャネルに入った Session を返す。
func Handshake(conn net.Conn, cfg LoginConfig) (*Session, *Hello, error) {
	conn.SetDeadline(time.Now().Add(connectingTimeout))
	defer conn.SetDeadline(time.Time{})

	br := bufio.NewReader(conn)
	host := conn.RemoteAddr().String()
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// signature (応答がそのまま Hello)
	if err := writeHTTP(conn, "/vpnsvc/connect.cgi", host, "image/jpeg", []byte("VPNCONNECT")); err != nil {
		return nil, nil, err
	}
	hp, err := readPackResponse(br)
	if err != nil {
		return nil, nil, fmt.Errorf("hello: %w", err)
	}
	if code := hp.GetInt("error"); code != 0 {
		return nil, nil, &ServerError{code}
	}
	hello := &Hello{Version: hp.GetInt("version"), Build: hp.GetInt("build")}
	hello.Server, _ = hp.GetStr("hello")
	hello.Random, _ = hp.GetData("random")
	if hello.Server == "" || len(hello.Random) != SHA0Size {
		return nil, nil, &ServerError{2}
	}

	// 認証
	if err := writeHTTP(conn, "/vpnsvc/vpn.cgi", host, "application/octet-stream", loginPack(cfg, hello.Random).Marshal()); err != nil {
		return nil, nil, err
	}
	var wp *Pack
	for i := 0; ; i++ {
		if wp, err = readPackResponse(br); err != nil {
			return nil, nil, fmt.Errorf("welcome: %w", err)
		}
		if wp.GetInt("noop") != 2 || i >= maxNoop {
			break
		}
	}
	if code := wp.GetInt("error"); code != 0 {
		return nil, nil, &ServerError{code}
	}
	if wp.GetInt("Redirect") != 0 {
		return nil, nil, errors.New("cluster redirect is not supported yet")
	}
	w, err := parseWelcome(wp)
	if err != nil {
		return nil, nil, err
	}
	if !w.UseEncrypt || w.UseCompress {
		return nil, nil, fmt.Errorf("unsupported session parameters (encrypt=%v compress=%v)", w.UseEncrypt, w.UseCompress)
	}
	return newSession(conn, br, w), hello, nil
}

func loginPack(cfg LoginConfig, random []byte) *Pack {
	p := NewPack()
	p.AddStr("method", "login")
	p.AddStr("hubname", cfg.Hub)
	p.AddStr("username", cfg.Username)
	p.AddInt("authtype", cfg.AuthType)
	if cfg.AuthType == AuthPassword {
		sp := SecurePassword(HashPassword(cfg.Username, cfg.Password), random)
		p.AddData("secure_password", sp[:])
	}
	// use_encrypt=0 だと TLS を外れて平文になるので必ず 1。TCP 1 本で使うため qos/half は 0
	p.AddInt("use_encrypt", 1)
	p.AddInt("use_compress", 0)
	p.AddInt("max_connection", 1)
	p.AddInt("half_connection", 0)
	p.AddBool("qos", false)
	p.AddInt("protocol", 0)

	p.AddStr("hello", clientStr)
	p.AddInt("version", clientVer)
	p.AddInt("build", clientBuild)
	p.AddStr("client_str", clientStr)
	p.AddInt("client_ver", clientVer)
	p.AddInt("client_build", clientBuild)
	p.AddInt("client_id", 0)

	uid := cfg.UniqueID
	if len(uid) != 20 {
		uid = make([]byte, 20)
		rand.Read(uid)
	}
	p.AddData("unique_id", uid)
	return p
}

func parseWelcome(p *Pack) (*Welcome, error) {
	w := &Welcome{
		MaxConnection: max(p.GetInt("max_connection"), 1),
		UseEncrypt:    p.GetBool("use_encrypt"),
		UseCompress:   p.GetBool("use_compress"),
		Timeout:       time.Duration(p.GetInt("timeout")) * time.Millisecond,
	}
	w.SessionName, _ = p.GetStr("session_name")
	w.ConnectionName, _ = p.GetStr("connection_name")
	w.SessionKey, _ = p.GetData("session_key")
	if msg, ok := p.GetData("Msg"); ok {
		w.Message = string(msg)
	}
	if w.SessionName == "" || w.ConnectionName == "" || len(w.SessionKey) != 20 {
		return nil, errors.New("invalid welcome pack")
	}
	if w.Timeout == 0 {
		w.Timeout = 30 * time.Second
	}
	return w, nil
}

func writeHTTP(w io.Writer, path, host, contentType string, body []byte) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "POST %s HTTP/1.1\r\n", path)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(http.TimeFormat))
	fmt.Fprintf(&b, "Host: %s\r\n", host)
	b.WriteString("Keep-Alive: timeout=15; max=19\r\nConnection: Keep-Alive\r\n")
	fmt.Fprintf(&b, "Content-Type: %s\r\n", contentType)
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n", len(body))
	b.Write(body)
	_, err := w.Write(b.Bytes())
	return err
}

// readPackResponse は HTTP レスポンスを 1 つ読み、ボディを PACK として返す。
// Content-Length 分だけ読むので、br に残ったバイトは後続 (データチャネル) のもの。
func readPackResponse(br *bufio.Reader) (*Pack, error) {
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %s", resp.Status)
	}
	n, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil || n <= 0 || n > 512<<20 {
		return nil, fmt.Errorf("invalid content-length %q", resp.Header.Get("Content-Length"))
	}
	return ReadPack(resp.Body, n)
}
