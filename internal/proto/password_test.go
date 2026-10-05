package proto

import (
	"encoding/base64"
	"testing"
)

func TestHashPassword(t *testing.T) {
	// SoftEther Server 5.01 が vpn_server.config に保存した AuthPassword (user=test, pass=testpass)
	const want = "GHwFtz8kQ8EKhGQYo+UxYv4AZFs="
	h := HashPassword("test", "testpass")
	if got := base64.StdEncoding.EncodeToString(h[:]); got != want {
		t.Errorf("HashPassword = %s, want %s", got, want)
	}
}
