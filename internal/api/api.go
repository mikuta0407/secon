// Package api はデーモンの制御 API (Unix ソケット上の HTTP+JSON) のサーバとクライアント。
package api

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"
)

// SystemSocket は root で動くデーモンのソケット。
const SystemSocket = "/var/run/secon.sock"

// DefaultSocket はこのプロセスの権限での既定ソケットパス。
func DefaultSocket() string {
	if os.Geteuid() == 0 {
		return SystemSocket
	}
	return UserSocket()
}

// UserSocket は一般ユーザで動くデーモン (SOCKS モードのみ) のソケット。
func UserSocket() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "secon.sock")
	}
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "secon", "secon.sock")
}

// ClientSocket は CLI / GUI が接続するソケットを決める。
// $SECON_SOCKET → システムデーモン → ユーザデーモン の順。
func ClientSocket() string {
	if s := os.Getenv("SECON_SOCKET"); s != "" {
		return s
	}
	if _, err := os.Stat(SystemSocket); err != nil {
		return UserSocket()
	}
	// root デーモンが異常終了するとソケットファイルだけ残るので、実際に繋がるか確かめる
	c, err := net.DialTimeout("unix", SystemSocket, time.Second)
	if err == nil {
		c.Close()
		return SystemSocket
	}
	if !errors.Is(err, fs.ErrPermission) {
		if _, err := os.Stat(UserSocket()); err == nil {
			return UserSocket()
		}
	}
	return SystemSocket // 権限エラーなどはそのまま案内する
}

// errorBody はエラー応答。
type errorBody struct {
	Error string `json:"error"`
}
