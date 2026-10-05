//go:build !darwin && !linux

package main

import "errors"

func autostartEnabled() bool { return false }

func setAutostart(on bool) error { return errors.New("autostart is not supported on this platform") }
