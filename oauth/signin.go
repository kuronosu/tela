package oauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela"
)

// callback is what the loopback listener received.
type callback struct {
	code  string
	err   error
	reply chan error // the sign-in's outcome, for the page shown in the browser
}

// SignIn signs the user in with the authorization code flow, PKCE and
// the system browser (RFC 8252, RFC 7636): it listens on 127.0.0.1 at a
// free port, opens the authorization URL with Config.Open, waits for the
// browser to come back to the listener with the code, checks the state,
// exchanges the code for tokens and stores them.
//
// It blocks until the user finishes in the browser, so run it off the UI
// thread (tela.UseGo, or [UseSession]) and give it a context with a
// timeout; canceling ctx closes the listener and returns ctx.Err(). The
// listener accepts one request that answers this sign-in and then closes:
// a redirect with another state fails the sign-in ([ErrStateMismatch]), and
// a server's refusal is an *[Error] ([ErrAccessDenied] for a user who said
// no).
//
// When the tokens could not be stored (desktop.ErrUnsupported without a
// keyring, ErrSecretCanceled), the error wraps that and the session stays
// signed in for this run.
func (s *Session) SignIn(ctx context.Context) error {
	cfg := &s.cfg
	if err := cfg.need("SignIn", "AuthURL", cfg.AuthURL); err != nil {
		return err
	}
	if err := cfg.need("SignIn", "TokenURL", cfg.TokenURL); err != nil {
		return err
	}
	verifier := randomString(32)
	state := randomString(24)

	// RFC 8252 §7.3: the loopback interface by IP literal, any free
	// port, and only that interface.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("oauth: listening for the redirect on 127.0.0.1: %w", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	host := fmt.Sprintf("127.0.0.1:%d", addr.Port)
	redirect := "http://" + host + cfg.redirectPath()

	cb := make(chan callback, 1)
	var once sync.Once
	finished := make(chan struct{}) // closed when the sign-in is over
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.redirectPath(), func(w http.ResponseWriter, r *http.Request) {
		s.handleRedirect(w, r, host, state, cb, &once, finished)
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute, // the answer waits for the code exchange
		MaxHeaderBytes:    16 << 10,
	}
	served := make(chan struct{})
	go func() { defer close(served); _ = srv.Serve(ln) }()
	var graceful bool
	defer func() {
		close(finished)
		if graceful {
			sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = srv.Shutdown(sctx)
		}
		_ = srv.Close()
		<-served
	}()

	authURL := s.authURL(redirect, state, challengeS256(verifier))
	open := cfg.Open
	if open == nil {
		open = func(u string) error { return openBrowser(ctx, u) }
	}
	if err := open(authURL); err != nil {
		return fmt.Errorf("oauth: opening the browser: %w", err)
	}

	var got callback
	select {
	case got = <-cb:
	case <-ctx.Done():
		return ctx.Err()
	}
	graceful = true
	if got.err != nil {
		got.reply <- got.err
		return got.err
	}

	tok, err := s.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {got.code},
		"redirect_uri":  {redirect},
		"code_verifier": {verifier},
	})
	if err != nil {
		err = fmt.Errorf("oauth: exchanging the code: %w", err)
		got.reply <- err
		return err
	}
	serr := s.signedIn(tok)
	got.reply <- serr
	return serr
}

// authURL builds the authorization request (RFC 6749 §4.1.1, RFC 7636
// §4.3).
func (s *Session) authURL(redirect, state, challenge string) string {
	cfg := &s.cfg
	u, _ := url.Parse(cfg.AuthURL) // validated
	q := u.Query()
	for k, v := range cfg.Params {
		q.Set(k, v)
	}
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirect)
	if len(cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String()
}

// handleRedirect answers one request of the loopback listener.
func (s *Session) handleRedirect(w http.ResponseWriter, r *http.Request, host, state string, cb chan callback, once *sync.Once, finished chan struct{}) {
	// A browser sends the host it was given; anything else is a request
	// that did not come from our redirect (DNS rebinding, a proxy).
	if r.Host != host {
		http.Error(w, "unexpected host", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	if !q.Has("code") && !q.Has("error") {
		http.Error(w, "this is not a sign-in redirect", http.StatusBadRequest)
		return
	}
	var got callback
	got.reply = make(chan error, 1)
	switch {
	case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1:
		got.err = ErrStateMismatch
	case q.Has("error"):
		got.err = &Error{Code: q.Get("error"), Description: q.Get("error_description"), URI: q.Get("error_uri")}
	default:
		got.code = q.Get("code")
		if got.code == "" {
			got.err = &Error{Code: "invalid_request", Description: "the redirect has an empty code"}
		}
	}
	accepted := false
	once.Do(func() { accepted = true; cb <- got })
	if !accepted {
		http.Error(w, "this sign-in already ended", http.StatusConflict)
		return
	}
	var outcome error
	select {
	case outcome = <-got.reply:
	case <-finished:
		outcome = errors.New("the sign-in was canceled")
	case <-r.Context().Done():
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if s.cfg.Page != nil {
		s.cfg.Page(w, outcome)
		return
	}
	defaultPage(w, outcome)
}

// defaultPage is the page the browser shows when the sign-in ends.
func defaultPage(w http.ResponseWriter, err error) {
	title, msg, status := "Signed in", "You can close this window and go back to the app.", http.StatusOK
	if err != nil {
		title, status = "Sign-in failed", http.StatusBadRequest
		msg = "The sign-in did not finish. You can close this window and try again in the app."
		if errors.Is(err, ErrAccessDenied) {
			msg = "Access was denied. You can close this window."
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%[1]s</title><style>body{font:16px system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;color:#222;background:#fafafa}main{text-align:center;padding:2rem}h1{font-size:1.4rem}</style></head><body><main><h1>%[1]s</h1><p>%[2]s</p></main></body></html>`, html.EscapeString(title), html.EscapeString(msg))
}

// openBrowser opens u with the system's default browser, on the UI thread
// of the running app.
func openBrowser(ctx context.Context, u string) error {
	res := make(chan error, 1)
	tela.Post(func() { res <- tela.OpenURL(u) })
	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
