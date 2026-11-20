package oauth_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/oauth"
	"github.com/kuronosu/tela/oauth/oauthtest"
	"github.com/kuronosu/tela/telatest"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tela-oauth-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv(desktop.DirEnv, dir)
	desktop.SetAppID("com.example.oauthtest")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// world is one test's server, running app (the secure storage needs one)
// and Config.
type world struct {
	t   *testing.T
	srv *oauthtest.Server
	app *telatest.App
	cfg oauth.Config
}

func newWorld(t *testing.T, opts ...oauthtest.Option) *world {
	t.Helper()
	srv := oauthtest.NewServer(opts...)
	t.Cleanup(srv.Close)
	app := telatest.MountFunc(t, func() tela.Node { return h.Div() }, telatest.Timeout(10*time.Second))
	return &world{t: t, srv: srv, app: app, cfg: srv.Config()}
}

func (w *world) session() *oauth.Session { return oauth.New(w.cfg) }

// stored returns the tokens in the secure storage.
func (w *world) stored() (oauth.Token, bool) {
	w.t.Helper()
	b, ok := w.app.StoredSecret(w.cfg.SecretName)
	if !ok {
		return oauth.Token{}, false
	}
	var tok oauth.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		w.t.Fatal(err)
	}
	return tok, true
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return c
}

// bg runs f on a goroutine of its own, as the secure storage's callers
// should, and returns its result.
func bg[T any](f func() T) T {
	ch := make(chan T, 1)
	go func() { ch <- f() }()
	return <-ch
}

type result[T any] struct {
	v   T
	err error
}

// call runs f on a goroutine of its own and returns its results.
func call[T any](f func() (T, error)) (T, error) {
	r := bg(func() result[T] { v, err := f(); return result[T]{v, err} })
	return r.v, r.err
}

// tokenOf returns s.Token's results.
func tokenOf(t *testing.T, s *oauth.Session) (oauth.Token, error) {
	t.Helper()
	return call(func() (oauth.Token, error) { return s.Token(ctx(t)) })
}

// fetch requests u with c and returns the status; the body is closed.
func fetch(c *http.Client, u string) (int, error) {
	return call(func() (int, error) {
		resp, err := c.Get(u)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	})
}

// closed reports whether nothing listens at host:port.
func closed(addr string) bool {
	for range 50 {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return true
		}
		c.Close()
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// listenerOf returns the host:port of the loopback listener a sign-in
// opened, from its authorization URL.
func listenerOf(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	r, err := url.Parse(u.Query().Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	return r.Host
}

// capture is a Config.Open that records the URL and does nothing else.
type capture struct {
	urls chan string
}

func newCapture() *capture { return &capture{urls: make(chan string, 4)} }

func (c *capture) open(u string) error { c.urls <- u; return nil }

func (c *capture) next(t *testing.T) string {
	t.Helper()
	select {
	case u := <-c.urls:
		return u
	case <-time.After(10 * time.Second):
		t.Fatal("the sign-in never opened the browser")
		return ""
	}
}
