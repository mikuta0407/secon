// relay は開発用の TCP 中継。vmnet の VM 間隔離を回避するため、
// ホスト上で listen してクライアント VM からの接続を vpn-server VM へ転送する。
//
//	go run ./dev/relay -map 192.168.65.1:8443=192.168.65.2:443 -map 192.168.65.1:3128=192.168.65.2:3128
package main

import (
	"flag"
	"io"
	"log"
	"net"
	"strings"
)

type mapFlag []string

func (m *mapFlag) String() string     { return strings.Join(*m, ",") }
func (m *mapFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	var maps mapFlag
	flag.Var(&maps, "map", "listen=target (複数可)")
	flag.Parse()
	for _, m := range maps {
		listen, target, ok := strings.Cut(m, "=")
		if !ok {
			log.Fatalf("bad -map %q", m)
		}
		ln, err := net.Listen("tcp", listen)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("relay %s -> %s", listen, target)
		go serve(ln, target)
	}
	select {}
}

func serve(ln net.Listener, target string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			defer c.Close()
			t, err := net.Dial("tcp", target)
			if err != nil {
				log.Print(err)
				return
			}
			defer t.Close()
			go io.Copy(t, c)
			io.Copy(c, t)
		}()
	}
}
