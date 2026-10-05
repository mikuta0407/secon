package proto

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestSHA0(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abc", "0164b8a914cd2a5e74c4f7ff082c4d97f1edf880"},
		{"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", "d2516ee1acfa5baf33dfc1c471e438449ef134c8"},
		{strings.Repeat("a", 1000000), "3232affa48628a26653b5aaa44541fd90d690603"},
	}
	for _, tt := range tests {
		got := SHA0([]byte(tt.in))
		if h := hex.EncodeToString(got[:]); h != tt.want {
			t.Errorf("SHA0(%.10q...) = %s, want %s", tt.in, h, tt.want)
		}
	}
}
