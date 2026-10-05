package i18n

import (
	"regexp"
	"testing"
)

func TestSwitch(t *testing.T) {
	defer Set(Auto)
	Set(Ja)
	if got := T("state.connected"); got != "接続済み" {
		t.Errorf("ja = %q", got)
	}
	Set(En)
	if got := T("state.connected"); got != "Connected" {
		t.Errorf("en = %q", got)
	}
}

func TestDetectEnv(t *testing.T) {
	t.Setenv("SECON_LANG", "ja_JP.UTF-8")
	if Detect() != Ja {
		t.Error("ja_JP should be Ja")
	}
	t.Setenv("SECON_LANG", "fr")
	if Detect() != En {
		t.Error("fr should fall back to En")
	}
}

var verb = regexp.MustCompile(`%[-+# 0]*[0-9]*[a-zA-Z]`)

// 英語と日本語で書式指定子の並びが一致していること (Sprintf の引数ずれ防止)。
func TestCatalogConsistency(t *testing.T) {
	for k, m := range messages {
		if m[0] == "" {
			t.Errorf("%s: missing English", k)
		}
		if m[1] == "" {
			continue
		}
		a, b := verb.FindAllString(m[0], -1), verb.FindAllString(m[1], -1)
		if len(a) != len(b) {
			t.Errorf("%s: verbs differ: %v vs %v", k, a, b)
			continue
		}
		for i := range a {
			if a[i] != b[i] {
				t.Errorf("%s: verb %d differs: %s vs %s", k, i, a[i], b[i])
			}
		}
	}
}
