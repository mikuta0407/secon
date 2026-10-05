package proto

import (
	"bytes"
	"testing"
)

func TestPackRoundTrip(t *testing.T) {
	p := NewPack()
	p.AddStr("method", "login")
	p.AddInt("max_connection", 1)
	p.AddInt64("big", 1<<40)
	p.AddData("random", []byte{1, 2, 3})
	p.AddUniStr("msg", "こんにちは")
	p.AddInt("MAX_connection", 2) // 大小無視で上書き

	q, err := UnmarshalPack(p.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := q.GetStr("METHOD"); s != "login" {
		t.Errorf("method = %q", s)
	}
	if v := q.GetInt("max_connection"); v != 2 {
		t.Errorf("max_connection = %d", v)
	}
	if v := q.GetInt64("big"); v != 1<<40 {
		t.Errorf("big = %d", v)
	}
	if d, _ := q.GetData("random"); !bytes.Equal(d, []byte{1, 2, 3}) {
		t.Errorf("random = %v", d)
	}
	if s, _ := q.GetUniStr("msg"); s != "こんにちは" {
		t.Errorf("msg = %q", s)
	}
	if v := q.GetInt("method"); v != 0 {
		t.Errorf("type mismatch should be 0, got %d", v)
	}
}

func TestPackWireFormat(t *testing.T) {
	p := NewPack()
	p.AddStr("ab", "xy")
	want := []byte{
		0, 0, 0, 1, // 要素数
		0, 0, 0, 3, 'a', 'b', // 名前 (長さ+1, NUL なし)
		0, 0, 0, 2, // STR
		0, 0, 0, 1, // 値数
		0, 0, 0, 2, 'x', 'y', // STR (長さそのまま)
	}
	if got := p.Marshal(); !bytes.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestPackRejectsDuplicate(t *testing.T) {
	p := NewPack()
	p.AddInt("a", 1)
	b := p.Marshal()
	// 要素を 2 回並べる
	dup := append([]byte{0, 0, 0, 2}, b[4:]...)
	dup = append(dup, b[4:]...)
	if _, err := UnmarshalPack(dup); err == nil {
		t.Error("expected duplicate error")
	}
}
