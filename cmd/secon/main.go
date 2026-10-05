// secon は SoftEther VPN クライアントの CLI。
package main

import (
	"fmt"
	"os"

	"github.com/mikuta0407/secon/internal/i18n"
)

// version はリリースビルドで -ldflags "-X main.version=..." により埋め込む。
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "status", "list", "ls":
		err = runStatus(args)
	case "connect", "up":
		err = runConnect(args)
	case "disconnect", "down":
		err = runDisconnect(args)
	case "reload":
		err = runReload(args)
	case "service":
		err = runService(args)
	case "daemon":
		err = runDaemon(args)
	case "debug":
		err = runDebug(args)
	case "version", "--version":
		fmt.Println("secon", version)
		return
	case "help", "-h", "--help":
		fmt.Print(i18n.T("cli.usage"))
		return
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "secon:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, i18n.T("cli.usage"))
	os.Exit(2)
}
