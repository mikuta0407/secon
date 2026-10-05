package proto

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"sync"
	"time"
)

const (
	keepAliveMagic   = 0xFFFFFFFF
	maxKeepAliveSize = 512
	// MaxFrameSize は送受信できる Ethernet フレームの最大長 (FCS なし)。
	MaxFrameSize   = 1600
	maxRecvBlock   = MaxFrameSize * 2
	maxBlocksPerTx = 256
)

// Session は認証後のデータチャネル。Ethernet フレームを送受信する。
// ReadFrame は 1 つの goroutine から、WriteFrames は並行に呼んでよい。
type Session struct {
	Welcome *Welcome

	conn net.Conn
	r    *bufio.Reader

	pending uint32 // 現在のデータヘッダで残っているブロック数
	hdr     [8]byte

	wmu     sync.Mutex
	wbuf    []byte
	closing chan struct{}
	once    sync.Once
}

func newSession(conn net.Conn, r *bufio.Reader, w *Welcome) *Session {
	s := &Session{Welcome: w, conn: conn, r: r, closing: make(chan struct{})}
	go s.keepAliveLoop()
	return s
}

// ReadFrame は次の Ethernet フレームを buf に読み込み、長さを返す。
// KeepAlive は内部で読み捨てる。Welcome.Timeout の間なにも届かなければエラー。
func (s *Session) ReadFrame(buf []byte) (int, error) {
	for {
		s.conn.SetReadDeadline(time.Now().Add(s.Welcome.Timeout))
		if s.pending == 0 {
			n, err := s.readU32()
			if err != nil {
				return 0, err
			}
			switch {
			case n == 0:
				continue
			case n == keepAliveMagic:
				size, err := s.readU32()
				if err != nil {
					return 0, err
				}
				if size > maxKeepAliveSize {
					return 0, fmt.Errorf("invalid keepalive size %d", size)
				}
				if _, err := s.r.Discard(int(size)); err != nil {
					return 0, err
				}
				continue
			default:
				s.pending = n
			}
		}

		size, err := s.readU32()
		if err != nil {
			return 0, err
		}
		s.pending--
		switch {
		case size > maxRecvBlock:
			return 0, fmt.Errorf("invalid block size %d", size)
		case size == 0:
			continue
		case int(size) > len(buf) || size > MaxFrameSize:
			if _, err := s.r.Discard(int(size)); err != nil {
				return 0, err
			}
			continue
		}
		if _, err := io.ReadFull(s.r, buf[:size]); err != nil {
			return 0, err
		}
		return int(size), nil
	}
}

func (s *Session) readU32() (uint32, error) {
	if _, err := io.ReadFull(s.r, s.hdr[:4]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(s.hdr[:4]), nil
}

// WriteFrames は Ethernet フレームをまとめて送信する。
func (s *Session) WriteFrames(frames ...[]byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	for len(frames) > 0 {
		chunk := frames[:min(len(frames), maxBlocksPerTx)]
		frames = frames[len(chunk):]
		b := binary.BigEndian.AppendUint32(s.wbuf[:0], uint32(len(chunk)))
		for _, f := range chunk {
			if len(f) > MaxFrameSize {
				return fmt.Errorf("frame too large (%d bytes)", len(f))
			}
			b = binary.BigEndian.AppendUint32(b, uint32(len(f)))
			b = append(b, f...)
		}
		s.wbuf = b
		if _, err := s.conn.Write(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) sendKeepAlive() error {
	size := mrand.IntN(maxKeepAliveSize)
	b := make([]byte, 8+size)
	binary.BigEndian.PutUint32(b, keepAliveMagic)
	binary.BigEndian.PutUint32(b[4:], uint32(size))
	rand.Read(b[8:])
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, err := s.conn.Write(b)
	return err
}

// keepAliveLoop は timeout/5〜timeout/2 のランダム間隔で KeepAlive を送る。
func (s *Session) keepAliveLoop() {
	t := s.Welcome.Timeout
	for {
		d := max(time.Duration(mrand.Int64N(int64(t/2))), t/5)
		select {
		case <-s.closing:
			return
		case <-time.After(d):
		}
		if err := s.sendKeepAlive(); err != nil {
			s.Close()
			return
		}
	}
}

// RemoteAddr は TCP の接続先 (プロキシ経由ならプロキシ) のアドレス。
func (s *Session) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }

// Close はセッションを切断する。
func (s *Session) Close() error {
	err := errors.New("already closed")
	s.once.Do(func() {
		close(s.closing)
		err = s.conn.Close()
	})
	return err
}
