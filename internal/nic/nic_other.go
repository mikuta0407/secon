//go:build !linux && !darwin

package nic

import (
	"errors"
	"net"
)

var errUnsupported = errors.New("nic mode is not supported on this platform yet")

func Open(name string, mac net.HardwareAddr, mtu int) (Device, error) { return nil, errUnsupported }

func Apply(dev Device, s Settings, logf func(string, ...any)) (Undo, error) {
	return nil, errUnsupported
}
