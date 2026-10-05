// secon は SoftEther VPN クライアントの CLI。
package main

import (
	"fmt"
	"os"
)

// version はリリースビルドで -ldflags "-X main.version=..." により埋め込む。
var version = "dev"

const usageText = `usage: secon <command> [args]

commands:
  status                 プロファイル一覧と接続状態
  connect <profile>      接続する
  disconnect <profile>   切断する
  reload                 デーモンに設定ファイルを再読み込みさせる
  service install|uninstall [--user]
                         launchd / systemd に登録・解除する
  daemon [flags]         デーモンとして起動する (通常は launchd / systemd から)
  version                バージョンを表示する
  debug dump|socks       開発用 (デーモンを使わずに直接接続する)
`

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
		fmt.Print(usageText)
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
	fmt.Fprint(os.Stderr, usageText)
	os.Exit(2)
}
