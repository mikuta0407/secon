package proto

import "strings"

// HashPassword は SoftEther のパスワードハッシュ SHA0(password || UPPER(username))。
// 大文字化はユーザ名の ASCII a-z のみ。
func HashPassword(username, password string) [SHA0Size]byte {
	upper := strings.Map(func(r rune) rune {
		if 'a' <= r && r <= 'z' {
			return r - 'a' + 'A'
		}
		return r
	}, username)
	return SHA0([]byte(password + upper))
}

// SecurePassword は Hello の random と組み合わせた認証値 SHA0(hashed || random)。
func SecurePassword(hashed [SHA0Size]byte, random []byte) [SHA0Size]byte {
	return SHA0(append(hashed[:], random...))
}
