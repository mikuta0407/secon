package main

import (
	"bufio"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

// instanceSocket は二重起動を防ぐためのソケット。2 つ目の secon-gui は
// 既存のものに「ウィンドウを出して」と伝えて終了する。
func instanceSocket() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "secon", "gui.sock")
}

// claimInstance は自分が唯一の GUI になれれば listener を返す。
// 既に動いている GUI があれば show を伝えて errAlreadyRunning を返す。
func claimInstance() (net.Listener, error) {
	path := instanceSocket()
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Write([]byte("show\n"))
		c.Close()
		return nil, errAlreadyRunning
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	os.Remove(path) // 前回の異常終了で残ったもの
	return net.Listen("unix", path)
}

var errAlreadyRunning = errors.New("secon-gui is already running")

// serveInstance は後から起動された GUI からの要求を受け、show なら onShow を呼ぶ。
func serveInstance(ln net.Listener, onShow func()) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		line, _ := bufio.NewReader(c).ReadString('\n')
		c.Close()
		if line == "show\n" {
			onShow()
		}
	}
}
