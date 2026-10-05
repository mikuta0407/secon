//go:build !darwin

package main

var onReopen func()

func hideDock() {}
func activate() {}
