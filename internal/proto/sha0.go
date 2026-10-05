package proto

import (
	"encoding/binary"
	"math/bits"
)

// SHA0Size は SHA-0 ダイジェストのバイト長。
const SHA0Size = 20

// SHA0 は SHA-0 (FIPS 180, 1993) を計算する。SoftEther のパスワードハッシュで使われる。
// SHA-1 との違いはメッセージスケジュールで 1 ビット左回転しない点だけ。
func SHA0(data []byte) [SHA0Size]byte {
	h := [5]uint32{0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0}

	msg := append([]byte(nil), data...)
	msg = append(msg, 0x80)
	for len(msg)%64 != 56 {
		msg = append(msg, 0)
	}
	msg = binary.BigEndian.AppendUint64(msg, uint64(len(data))*8)

	var w [80]uint32
	for off := 0; off < len(msg); off += 64 {
		for i := 0; i < 16; i++ {
			w[i] = binary.BigEndian.Uint32(msg[off+i*4:])
		}
		for i := 16; i < 80; i++ {
			w[i] = w[i-3] ^ w[i-8] ^ w[i-14] ^ w[i-16]
		}
		a, b, c, d, e := h[0], h[1], h[2], h[3], h[4]
		for i := 0; i < 80; i++ {
			var f, k uint32
			switch {
			case i < 20:
				f, k = (b&c)|(^b&d), 0x5A827999
			case i < 40:
				f, k = b^c^d, 0x6ED9EBA1
			case i < 60:
				f, k = (b&c)|(b&d)|(c&d), 0x8F1BBCDC
			default:
				f, k = b^c^d, 0xCA62C1D6
			}
			t := bits.RotateLeft32(a, 5) + f + e + k + w[i]
			a, b, c, d, e = t, a, bits.RotateLeft32(b, 30), c, d
		}
		h[0] += a
		h[1] += b
		h[2] += c
		h[3] += d
		h[4] += e
	}

	var out [SHA0Size]byte
	for i, v := range h {
		binary.BigEndian.PutUint32(out[i*4:], v)
	}
	return out
}
