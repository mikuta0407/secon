package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mikuta0407/secon/internal/api"
	"github.com/mikuta0407/secon/internal/engine"
)

func client() *api.Client { return api.NewClient(api.ClientSocket()) }

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func runStatus(args []string) error {
	c, cancel := ctx()
	defer cancel()
	st, err := client().Status(c)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		for _, s := range st {
			if s.Name == args[0] {
				printDetail(s)
				return nil
			}
		}
		return fmt.Errorf("no such profile: %s", args[0])
	}
	if len(st) == 0 {
		fmt.Println("プロファイルがありません")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tMODE\tSTATE\tADDRESS\tSERVER")
	for _, s := range st {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.Mode, s.State, dash(s.Address), s.Server)
	}
	return tw.Flush()
}

func printDetail(s engine.Status) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "%s:\t%s\n", k, v)
		}
	}
	row("Name", s.Name)
	row("Mode", s.Mode)
	row("Server", s.Server)
	row("Hub/User", s.Hub+" / "+s.User)
	row("State", fmt.Sprintf("%s (since %s)", s.State, s.Since.Local().Format(time.DateTime)))
	row("Error", s.Error)
	row("Session", s.Session)
	row("Server info", s.ServerInfo)
	row("Interface", s.Interface)
	row("Address", s.Address)
	row("Gateway", s.Gateway)
	row("DNS", strings.Join(s.DNS, ", "))
	row("SOCKS5", s.Socks)
	for _, f := range s.Forwards {
		row("Forward", f)
	}
	if s.State == engine.StateConnected {
		row("Uptime", time.Since(s.Since).Round(time.Second).String())
		row("Received", fmt.Sprintf("%d bytes (%d packets)", s.BytesIn, s.PacketsIn))
		row("Sent", fmt.Sprintf("%d bytes (%d packets)", s.BytesOut, s.PacketsOut))
	}
	tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func runConnect(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: secon connect <profile>")
	}
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cl := client()
	if err := cl.Connect(c, args[0]); err != nil {
		return err
	}
	// 接続完了 (またはエラー) まで待つ
	var final *engine.Status
	err := cl.Events(c, func(st []engine.Status) {
		for _, s := range st {
			if s.Name == args[0] && (s.State == engine.StateConnected || s.State == engine.StateFailed || s.Error != "") {
				final = &s
				cancel()
			}
		}
	})
	if final == nil {
		if err == nil || c.Err() != nil {
			return fmt.Errorf("timed out waiting for connection (still trying in background)")
		}
		return err
	}
	if final.State != engine.StateConnected {
		return fmt.Errorf("%s: %s (daemon keeps retrying; 'secon disconnect %s' to stop)", final.State, final.Error, final.Name)
	}
	fmt.Printf("connected: %s (%s)\n", final.Name, final.Address)
	return nil
}

func runDisconnect(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: secon disconnect <profile>")
	}
	c, cancel := ctx()
	defer cancel()
	return client().Disconnect(c, args[0])
}

func runReload(args []string) error {
	c, cancel := ctx()
	defer cancel()
	return client().Reload(c)
}
