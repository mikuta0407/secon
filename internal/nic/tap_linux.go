package nic

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

type tap struct {
	name string
	f    *os.File
}

// Open は TAP デバイスを作成し、MAC と MTU を設定して up する。
func Open(name string, mac net.HardwareAddr, mtu int) (Device, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	ifr.SetUint16(unix.IFF_TAP | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("TUNSETIFF %s: %w", name, err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	t := &tap{name: ifr.Name(), f: os.NewFile(uintptr(fd), "/dev/net/tun")}
	if err := linkSetup(t.name, mac, mtu); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
}

func (t *tap) Name() string                    { return t.name }
func (t *tap) ReadFrame(b []byte) (int, error) { return t.f.Read(b) }
func (t *tap) WriteFrame(b []byte) error       { _, err := t.f.Write(b); return err }
func (t *tap) Close() error                    { return t.f.Close() }
