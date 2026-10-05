package nic

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/mikuta0407/secon/internal/l2"
)

const (
	utunControl     = "com.apple.net.utun_control"
	sysprotoControl = 2 // SYSPROTO_CONTROL (sys/kern_control.h)
)

// utun は macOS の utun (L3) に L2 エミュレーションを被せた Device。
type utun struct {
	name string
	f    *os.File
	emu  *l2.Emulator
	in   chan []byte // utun から読んだ IPv4 パケット
	errc chan error
	done chan struct{} // Close で閉じる (readLoop が in への送信で止まったままにならないように)
}

// Open は utun を作成する (名前はカーネルが utunN を割り当てるので name は使わない)。
func Open(_ string, mac net.HardwareAddr, mtu int) (Device, error) {
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, sysprotoControl)
	if err != nil {
		return nil, fmt.Errorf("utun socket: %w", err)
	}
	info := &unix.CtlInfo{}
	copy(info.Name[:], utunControl)
	if err := unix.IoctlCtlInfo(fd, info); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("CTLIOCGINFO: %w", err)
	}
	if err := unix.Connect(fd, &unix.SockaddrCtl{ID: info.Id, Unit: 0}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("utun connect: %w", err)
	}
	name, err := unix.GetsockoptString(fd, sysprotoControl, 2 /* UTUN_OPT_IFNAME */)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	u := &utun{
		name: name,
		f:    os.NewFile(uintptr(fd), name),
		emu:  l2.NewEmulator(mac),
		in:   make(chan []byte, 256),
		errc: make(chan error, 1),
		done: make(chan struct{}),
	}
	if out, err := exec.Command("ifconfig", name, "mtu", strconv.Itoa(mtu), "up").CombinedOutput(); err != nil {
		u.Close()
		return nil, fmt.Errorf("ifconfig: %v: %s", err, out)
	}
	go u.readLoop()
	return u, nil
}

func (u *utun) Name() string { return u.name }

func (u *utun) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, err := u.f.Read(buf)
		if err != nil {
			u.errc <- err
			close(u.in)
			return
		}
		// 先頭 4 バイトはアドレスファミリ (ネットワークバイトオーダ)
		if n <= 4 || binary.BigEndian.Uint32(buf[:4]) != unix.AF_INET {
			continue
		}
		select {
		case u.in <- append([]byte(nil), buf[4:n]...):
		case <-u.done:
			return
		}
	}
}

// ReadFrame は VPN へ送る Ethernet フレームを返す。
func (u *utun) ReadFrame(b []byte) (int, error) {
	for {
		select {
		case f := <-u.emu.Out():
			return copy(b, f), nil
		case pkt, ok := <-u.in:
			if !ok {
				return 0, <-u.errc
			}
			if f := u.emu.ToVPN(pkt); f != nil {
				return copy(b, f), nil
			}
		}
	}
}

// WriteFrame は VPN から来たフレームを処理し、IPv4 なら utun に書く。
func (u *utun) WriteFrame(f []byte) error {
	pkt := u.emu.FromVPN(f)
	if pkt == nil {
		return nil
	}
	b := make([]byte, 4+len(pkt))
	binary.BigEndian.PutUint32(b, unix.AF_INET)
	copy(b[4:], pkt)
	_, err := u.f.Write(b)
	return err
}

func (u *utun) Close() error {
	select {
	case <-u.done:
	default:
		close(u.done)
	}
	return u.f.Close()
}

func (u *utun) setIPv4(addr netip.Prefix, gw netip.Addr) { u.emu.SetIPv4(addr, gw) }
