// Package i18n は CLI / GUI の表示言語 (英語・日本語) を扱う。
//
// 言語は Set で選ぶ。Auto なら $SECON_LANG、OS のロケールの順に判別し、日本語以外は英語にする。
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/jeandeaual/go-locale"
)

// Lang は表示言語。
type Lang string

const (
	Auto Lang = "auto"
	En   Lang = "en"
	Ja   Lang = "ja"
)

var current atomic.Value // Lang (En か Ja)

func init() { Set(Auto) }

// Set は表示言語を設定する。Auto は自動判別。
func Set(l Lang) {
	if l != En && l != Ja {
		l = Detect()
	}
	current.Store(l)
}

// Current は現在の表示言語 (En か Ja)。
func Current() Lang { return current.Load().(Lang) }

// Detect は $SECON_LANG と OS のロケールから言語を決める。
func Detect() Lang {
	if v := os.Getenv("SECON_LANG"); v != "" {
		return normalize(v)
	}
	// macOS の Finder 起動など $LANG が無い場合も go-locale は OS 設定から取れる
	if l, err := locale.GetLanguage(); err == nil && l != "" {
		return normalize(l)
	}
	return En
}

func normalize(s string) Lang {
	if strings.HasPrefix(strings.ToLower(s), "ja") {
		return Ja
	}
	return En
}

// T はメッセージ key を現在の言語で返す。args があれば fmt.Sprintf で埋め込む。
func T(key string, args ...any) string {
	m, ok := messages[key]
	if !ok {
		panic("i18n: unknown message key " + key)
	}
	s := m[0]
	if Current() == Ja && m[1] != "" {
		s = m[1]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Errorf は翻訳済みのメッセージでエラーを作る。
func Errorf(key string, args ...any) error {
	return fmt.Errorf("%s", T(key, args...))
}
