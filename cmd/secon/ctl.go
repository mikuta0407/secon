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
	"github.com/mikuta0407/secon/internal/i18n"
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
		return i18n.Errorf("cli.noSuchProfile", args[0])
	}
	if len(st) == 0 {
		fmt.Println(i18n.T("cli.noProfiles"))
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
	row := func(key, v string) {
		if v != "" {
			fmt.Fprintf(tw, "%s:\t%s\n", i18n.T(key), v)
		}
	}
	fmt.Fprintf(tw, "%s:\t%s\n", i18n.T("col.name"), s.Name)
	row("detail.mode", s.Mode)
	row("detail.server", s.Server)
	row("detail.hubUser", s.Hub+" / "+s.User)
	row("detail.state", fmt.Sprintf("%s (%s)", s.State, s.Since.Local().Format(time.DateTime)))
	row("detail.error", s.Error)
	row("detail.session", s.Session)
	row("detail.serverInfo", s.ServerInfo)
	row("detail.interface", s.Interface)
	row("detail.address", s.Address)
	row("detail.gateway", s.Gateway)
	row("detail.dns", strings.Join(s.DNS, ", "))
	row("detail.socks", s.Socks)
	for _, f := range s.Forwards {
		row("detail.forward", f)
	}
	if s.State == engine.StateConnected {
		row("detail.uptime", time.Since(s.Since).Round(time.Second).String())
		row("detail.received", i18n.T("detail.traffic", fmt.Sprintf("%d bytes", s.BytesIn), s.PacketsIn))
		row("detail.sent", i18n.T("detail.traffic", fmt.Sprintf("%d bytes", s.BytesOut), s.PacketsOut))
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
			return i18n.Errorf("cli.waitTimeout")
		}
		return err
	}
	if final.State != engine.StateConnected {
		return i18n.Errorf("cli.connectFailed", final.State, final.Error, final.Name)
	}
	fmt.Println(i18n.T("cli.connected", final.Name, final.Address))
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
