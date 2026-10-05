// Package engine はプロファイルごとの VPN 接続を管理する (接続・再接続・リスナ)。
package engine

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/mikuta0407/secon/internal/config"
)

// State は接続状態。
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateReconnecting State = "reconnecting"
	StateFailed       State = "failed" // 設定エラーなど、再試行しても直らない
)

// Status はプロファイルの状態 (API でそのまま返す)。
type Status struct {
	Name        string    `json:"name"`
	Mode        string    `json:"mode"`
	Server      string    `json:"server"`
	Hub         string    `json:"hub"`
	User        string    `json:"user"`
	State       State     `json:"state"`
	Error       string    `json:"error,omitempty"`
	Since       time.Time `json:"since"`
	Session     string    `json:"session,omitempty"`
	ServerInfo  string    `json:"server_info,omitempty"` // サーバ製品名・ビルド
	Interface   string    `json:"interface,omitempty"`   // NIC モードの仮想 NIC 名
	BytesIn     uint64    `json:"bytes_in,omitempty"`
	BytesOut    uint64    `json:"bytes_out,omitempty"`
	PacketsIn   uint64    `json:"packets_in,omitempty"`
	PacketsOut  uint64    `json:"packets_out,omitempty"`
	Address     string    `json:"address,omitempty"`
	Gateway     string    `json:"gateway,omitempty"`
	DNS         []string  `json:"dns,omitempty"`
	Socks       string    `json:"socks,omitempty"`
	Forwards    []string  `json:"forwards,omitempty"`
	AutoConnect bool      `json:"auto_connect"`
}

// ErrNoProfile は存在しないプロファイル名。
var ErrNoProfile = errors.New("no such profile")

// Manager は全プロファイルを管理する。
type Manager struct {
	Logf func(format string, args ...any)

	mu       sync.Mutex
	profiles map[string]*runner
	order    []string
	notify   []chan struct{}
}

func NewManager() *Manager {
	return &Manager{Logf: log.Printf, profiles: map[string]*runner{}}
}

// Apply は設定を反映する。変更・削除されたプロファイルは切断し、auto_connect のものは接続する。
func (m *Manager) Apply(cfg *config.Config) {
	m.mu.Lock()
	next := map[string]*runner{}
	var order []string
	var stopping []*runner
	var starting []*runner
	for _, p := range cfg.Profiles {
		order = append(order, p.Name)
		if r, ok := m.profiles[p.Name]; ok && reflect.DeepEqual(r.profile, p) {
			next[p.Name] = r
			continue
		}
		if r, ok := m.profiles[p.Name]; ok {
			stopping = append(stopping, r)
		}
		r := newRunner(p, m)
		next[p.Name] = r
		if p.AutoConnect {
			starting = append(starting, r)
		}
	}
	for name, r := range m.profiles {
		if _, ok := next[name]; !ok {
			stopping = append(stopping, r)
		}
	}
	m.profiles, m.order = next, order
	m.changed()
	m.mu.Unlock()

	// 停止中の runner は状態通知のため m.mu を取るので、ロックの外で待つ
	var wg sync.WaitGroup
	for _, r := range stopping {
		wg.Add(1)
		go func() { defer wg.Done(); r.stop() }()
	}
	wg.Wait()
	for _, r := range starting {
		r.start()
	}
}

func (m *Manager) get(name string) (*runner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.profiles[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoProfile, name)
	}
	return r, nil
}

// Connect はプロファイルを接続状態にする (既に接続中なら何もしない)。
func (m *Manager) Connect(name string) error {
	r, err := m.get(name)
	if err != nil {
		return err
	}
	r.start()
	return nil
}

// Disconnect はプロファイルを切断する。
func (m *Manager) Disconnect(name string) error {
	r, err := m.get(name)
	if err != nil {
		return err
	}
	r.stop()
	return nil
}

// Status は全プロファイルの状態を設定ファイルの順に返す。
func (m *Manager) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.order))
	for _, name := range m.order {
		out = append(out, m.profiles[name].getStatus())
	}
	return out
}

// Subscribe は状態変化のたびに通知を受け取るチャネルを返す (取りこぼしはまとめられる)。
func (m *Manager) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	m.mu.Lock()
	m.notify = append(m.notify, ch)
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, c := range m.notify {
			if c == ch {
				m.notify = append(m.notify[:i], m.notify[i+1:]...)
				break
			}
		}
	}
}

// changed は購読者に通知する (m.mu を保持して呼ぶ)。
func (m *Manager) changed() {
	for _, c := range m.notify {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) notifyChanged() {
	m.mu.Lock()
	m.changed()
	m.mu.Unlock()
}

// Close は全プロファイルを切断する。
func (m *Manager) Close() {
	m.mu.Lock()
	rs := make([]*runner, 0, len(m.profiles))
	for _, r := range m.profiles {
		rs = append(rs, r)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, r := range rs {
		wg.Add(1)
		go func() { defer wg.Done(); r.stop() }()
	}
	wg.Wait()
}

// Names はプロファイル名一覧 (補完用)。
func (m *Manager) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := append([]string(nil), m.order...)
	sort.Strings(names)
	return names
}
