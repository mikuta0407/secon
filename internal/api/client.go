package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/mikuta0407/secon/internal/config"
	"github.com/mikuta0407/secon/internal/engine"
)

// Client は制御 API のクライアント。
type Client struct {
	Socket string
	hc     *http.Client
}

func NewClient(socket string) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}
	return &Client{Socket: socket, hc: &http.Client{Transport: tr}}
}

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	return c.doBody(ctx, method, path, nil, out)
}

func (c *Client) doBody(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://secon"+path, body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return c.dialError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e errorBody
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("daemon: %s", resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Status(ctx context.Context) ([]engine.Status, error) {
	var st []engine.Status
	return st, c.do(ctx, http.MethodGet, "/v1/status", &st)
}

func (c *Client) Connect(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/profiles/"+url.PathEscape(name)+"/connect", nil)
}

func (c *Client) Disconnect(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/profiles/"+url.PathEscape(name)+"/disconnect", nil)
}

func (c *Client) Reload(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/v1/reload", nil)
}

func (c *Client) dialError(err error) error {
	switch {
	case errors.Is(err, fs.ErrPermission):
		hint := "add your user to the socket's group"
		if fi, serr := os.Stat(c.Socket); serr == nil {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				if g, gerr := user.LookupGroupId(strconv.Itoa(int(st.Gid))); gerr == nil {
					hint = fmt.Sprintf("add your user to group %q (or set api.group in the config)", g.Name)
				}
			}
		}
		return fmt.Errorf("permission denied on %s: %s", c.Socket, hint)
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ECONNREFUSED):
		return fmt.Errorf("daemon is not running (%s); start it with 'secon service install'", c.Socket)
	}
	return fmt.Errorf("cannot reach daemon (%s): %w", c.Socket, err)
}

// Profiles は設定済みプロファイル (パスワードを含む) を返す。
func (c *Client) Profiles(ctx context.Context) ([]config.Profile, error) {
	var ps []config.Profile
	return ps, c.do(ctx, http.MethodGet, "/v1/config/profiles", &ps)
}

// PutProfile はプロファイルを保存する。oldName が空なら追加、そうでなければ置き換え (改名可)。
func (c *Client) PutProfile(ctx context.Context, oldName string, p config.Profile) error {
	if oldName == "" {
		return c.doBody(ctx, http.MethodPost, "/v1/config/profiles", p, nil)
	}
	return c.doBody(ctx, http.MethodPut, "/v1/config/profiles/"+url.PathEscape(oldName), p, nil)
}

func (c *Client) DeleteProfile(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/config/profiles/"+url.PathEscape(name), nil)
}

// Events は状態変化のたびに fn を呼ぶ。ctx が終わるか接続が切れると戻る。
func (c *Client) Events(ctx context.Context, fn func([]engine.Status)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://secon/v1/events", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return c.dialError(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var st []engine.Status
		if err := json.Unmarshal([]byte(data), &st); err == nil {
			fn(st)
		}
	}
	return sc.Err()
}
