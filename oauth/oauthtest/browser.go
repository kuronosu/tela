package oauthtest

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Browser returns a fake browser, for oauth.Config.Open: it follows the
// authorization URL of this server, and the redirect it answers with, to
// the app's loopback listener, as a user's browser does once they approve.
// It runs the visit in the background, as a real browser is a separate
// process, and keeps the page the app showed ([Server.Page]).
//
// A URL that is not this server's is an error: the app must not send its
// user elsewhere.
func (s *Server) Browser() func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		base, _ := url.Parse(s.URL)
		if u.Scheme != base.Scheme || u.Host != base.Host {
			return fmt.Errorf("oauthtest: the fake browser got %q, not an URL of the server (%s)", u.Redacted(), s.URL)
		}
		go s.visit(raw)
		return nil
	}
}

// visit GETs raw like a browser (following redirects) and records the
// final page.
func (s *Server) visit(raw string) {
	c := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{}}
	defer c.CloseIdleConnections()
	resp, err := c.Get(raw)
	if err != nil {
		s.pages <- page{status: 0, body: err.Error()}
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	s.pages <- page{status: resp.StatusCode, body: string(body)}
}

// Page returns the status and body of the page the app showed in the
// browser at the end of the oldest visit not yet read, waiting a few
// seconds for it. A status of 0 means the visit failed, and the body is
// the error. ok is false when no visit finished.
func (s *Server) Page() (status int, body string, ok bool) {
	select {
	case p := <-s.pages:
		return p.status, p.body, true
	case <-time.After(5 * time.Second):
		return 0, "", false
	}
}
