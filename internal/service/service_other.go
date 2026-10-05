//go:build !linux && !darwin

package service

import "errors"

var errUnsupported = errors.New("service management is not supported on this platform")

func Install(o Options) (string, error)   { return "", errUnsupported }
func Uninstall(o Options) (string, error) { return "", errUnsupported }
