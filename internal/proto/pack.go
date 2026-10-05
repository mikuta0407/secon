package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// PACK の値型。docs/PROTOCOL.md §2 参照。
const (
	TypeInt    uint32 = 0
	TypeData   uint32 = 1
	TypeStr    uint32 = 2
	TypeUniStr uint32 = 3
	TypeInt64  uint32 = 4
)

const (
	maxElementNameLen = 63
	maxPackElements   = 262144
	maxPackValues     = 262144
	maxPackDataSize   = 384 << 20
)

// Element は PACK の 1 要素。Values の各要素の Go 型は Type に対応する
// (Int: uint32, Int64: uint64, Data: []byte, Str/UniStr: string)。
type Element struct {
	Name   string
	Type   uint32
	Values []any
}

// Pack は SoftEther の PACK (名前付き要素の集合)。名前は大小文字無視で一意。
type Pack struct {
	elems []*Element
}

func NewPack() *Pack { return &Pack{} }

func (p *Pack) find(name string) *Element {
	for _, e := range p.elems {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

// set は同名要素を置き換える (同名重複はサーバに PACK ごと拒否されるため)。
func (p *Pack) set(name string, typ uint32, v any) {
	if e := p.find(name); e != nil {
		e.Type, e.Values = typ, []any{v}
		return
	}
	p.elems = append(p.elems, &Element{Name: name, Type: typ, Values: []any{v}})
}

func (p *Pack) AddInt(name string, v uint32)    { p.set(name, TypeInt, v) }
func (p *Pack) AddInt64(name string, v uint64)  { p.set(name, TypeInt64, v) }
func (p *Pack) AddData(name string, v []byte)   { p.set(name, TypeData, v) }
func (p *Pack) AddStr(name string, v string)    { p.set(name, TypeStr, v) }
func (p *Pack) AddUniStr(name string, v string) { p.set(name, TypeUniStr, v) }

func (p *Pack) AddBool(name string, v bool) {
	var i uint32
	if v {
		i = 1
	}
	p.AddInt(name, i)
}

func (p *Pack) value(name string, typ uint32) (any, bool) {
	e := p.find(name)
	if e == nil || e.Type != typ || len(e.Values) == 0 {
		return nil, false
	}
	return e.Values[0], true
}

// GetInt は INT 要素を返す。無い・型違いなら 0 (本家と同じ扱い)。
func (p *Pack) GetInt(name string) uint32 {
	v, _ := p.value(name, TypeInt)
	i, _ := v.(uint32)
	return i
}

func (p *Pack) GetBool(name string) bool { return p.GetInt(name) != 0 }

func (p *Pack) GetInt64(name string) uint64 {
	v, _ := p.value(name, TypeInt64)
	i, _ := v.(uint64)
	return i
}

func (p *Pack) GetData(name string) ([]byte, bool) {
	v, ok := p.value(name, TypeData)
	b, _ := v.([]byte)
	return b, ok
}

func (p *Pack) GetStr(name string) (string, bool) {
	v, ok := p.value(name, TypeStr)
	s, _ := v.(string)
	return s, ok
}

func (p *Pack) GetUniStr(name string) (string, bool) {
	v, ok := p.value(name, TypeUniStr)
	s, _ := v.(string)
	return s, ok
}

// Elements は要素一覧を返す (デバッグ用)。
func (p *Pack) Elements() []*Element { return p.elems }

// Marshal は PACK をバイト列にする。
func (p *Pack) Marshal() []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(p.elems)))
	for _, e := range p.elems {
		// 名前は「長さ+1」を書くが NUL は書かない
		b = binary.BigEndian.AppendUint32(b, uint32(len(e.Name)+1))
		b = append(b, e.Name...)
		b = binary.BigEndian.AppendUint32(b, e.Type)
		b = binary.BigEndian.AppendUint32(b, uint32(len(e.Values)))
		for _, v := range e.Values {
			switch e.Type {
			case TypeInt:
				b = binary.BigEndian.AppendUint32(b, v.(uint32))
			case TypeInt64:
				b = binary.BigEndian.AppendUint64(b, v.(uint64))
			case TypeData:
				d := v.([]byte)
				b = binary.BigEndian.AppendUint32(b, uint32(len(d)))
				b = append(b, d...)
			case TypeStr:
				s := v.(string)
				b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
				b = append(b, s...)
			case TypeUniStr:
				// UNISTR は長さ+1 で末尾 NUL も書く
				s := v.(string)
				b = binary.BigEndian.AppendUint32(b, uint32(len(s)+1))
				b = append(b, s...)
				b = append(b, 0)
			}
		}
	}
	return b
}

var errPackTruncated = errors.New("pack: truncated")

type packReader struct{ b []byte }

func (r *packReader) u32() (uint32, error) {
	if len(r.b) < 4 {
		return 0, errPackTruncated
	}
	v := binary.BigEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v, nil
}

func (r *packReader) u64() (uint64, error) {
	if len(r.b) < 8 {
		return 0, errPackTruncated
	}
	v := binary.BigEndian.Uint64(r.b)
	r.b = r.b[8:]
	return v, nil
}

func (r *packReader) bytes(n uint32) ([]byte, error) {
	if uint64(len(r.b)) < uint64(n) {
		return nil, errPackTruncated
	}
	v := r.b[:n:n]
	r.b = r.b[n:]
	return v, nil
}

// UnmarshalPack はバイト列を PACK にする。
func UnmarshalPack(data []byte) (*Pack, error) {
	r := &packReader{b: data}
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if n > maxPackElements {
		return nil, fmt.Errorf("pack: too many elements (%d)", n)
	}
	p := NewPack()
	for i := uint32(0); i < n; i++ {
		nameLen, err := r.u32()
		if err != nil {
			return nil, err
		}
		if nameLen == 0 {
			return nil, errors.New("pack: invalid element name length")
		}
		name, err := r.bytes(nameLen - 1)
		if err != nil {
			return nil, err
		}
		if len(name) > maxElementNameLen {
			name = name[:maxElementNameLen]
		}
		typ, err := r.u32()
		if err != nil {
			return nil, err
		}
		num, err := r.u32()
		if err != nil {
			return nil, err
		}
		if num == 0 || num > maxPackValues {
			return nil, fmt.Errorf("pack: element %q has invalid value count %d", name, num)
		}
		e := &Element{Name: string(name), Type: typ, Values: make([]any, 0, num)}
		for j := uint32(0); j < num; j++ {
			var v any
			switch typ {
			case TypeInt:
				v, err = r.u32()
			case TypeInt64:
				v, err = r.u64()
			case TypeData, TypeStr, TypeUniStr:
				var size uint32
				if size, err = r.u32(); err != nil {
					break
				}
				if size > maxPackDataSize {
					return nil, fmt.Errorf("pack: element %q too large", name)
				}
				var d []byte
				if d, err = r.bytes(size); err != nil {
					break
				}
				switch typ {
				case TypeData:
					v = append([]byte(nil), d...)
				case TypeStr:
					v = string(d)
				default:
					v = strings.TrimRight(string(d), "\x00")
				}
			default:
				return nil, fmt.Errorf("pack: element %q has unknown type %d", name, typ)
			}
			if err != nil {
				return nil, err
			}
			e.Values = append(e.Values, v)
		}
		if p.find(e.Name) != nil {
			return nil, fmt.Errorf("pack: duplicate element %q", e.Name)
		}
		p.elems = append(p.elems, e)
	}
	return p, nil
}

// ReadPack は r から size バイト読んで PACK にする。
func ReadPack(r io.Reader, size int) (*Pack, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return UnmarshalPack(buf)
}
