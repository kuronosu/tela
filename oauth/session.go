package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/kuronosu/tela/desktop"
)

// Session is one account of a [Config]: its tokens, in memory and in the
// secure storage, and the calls that sign in, use and forget them. It is
// safe for concurrent use. Make one per Config.SecretName (two Sessions of
// the same secret do not see each other's changes); components get theirs
// from [UseSession], through [State.Session].
type Session struct {
	cfg Config

	mu        sync.Mutex
	loaded    bool
	tok       *Token
	gen       int // changes whenever the account does, so a late refresh is dropped
	flight    *flight
	listeners []func(signedIn bool)
}

// flight is one refresh in progress, shared by the callers that wait for
// it.
type flight struct {
	done chan struct{}
	tok  Token
	err  error
}

// New returns the Session of cfg. It reports a Config that cannot work
// (a missing client ID, an http endpoint) as a panic with the fix, like the
// other constructors of Tela.
func New(cfg Config) *Session {
	if err := cfg.validate(); err != nil {
		panic(err.Error())
	}
	return &Session{cfg: cfg}
}

// refreshTimeout bounds one refresh request: it is shared, so the
// context of one caller does not govern it.
const refreshTimeout = 30 * time.Second

// SignedIn reports whether the session holds tokens, loading them from the
// secure storage the first time. It does not contact the server: an access
// token that has expired counts, since Token refreshes it. The error is
// the secure storage's (desktop.ErrSecretCanceled when the user dismissed
// the unlock prompt; ask again later).
func (s *Session) SignedIn(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return false, err
	}
	return s.tok != nil, nil
}

// loadLocked reads the stored tokens once. A desktop without secure
// storage works with the tokens in memory only.
func (s *Session) loadLocked() error {
	if s.loaded {
		return nil
	}
	b, err := desktop.Secret(s.cfg.secretName())
	switch {
	case errors.Is(err, desktop.ErrSecretNotFound), errors.Is(err, desktop.ErrUnsupported):
		s.loaded = true
		return nil
	case err != nil:
		return fmt.Errorf("oauth: reading the stored tokens: %w", err)
	}
	s.loaded = true
	var t Token
	if json.Unmarshal(b, &t) != nil || t.AccessToken == "" {
		return nil // not ours, or damaged: signed out, and the next sign-in replaces it
	}
	s.tok = &t
	return nil
}

// persist stores t in the secure storage.
func (s *Session) persist(t Token) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := desktop.SetSecret(s.cfg.secretName(), b); err != nil {
		return fmt.Errorf("oauth: storing the tokens: %w", err)
	}
	return nil
}

// signedIn replaces the account's tokens after a sign-in. The tokens stay
// in memory when they cannot be stored; the error says so.
func (s *Session) signedIn(t Token) error {
	s.mu.Lock()
	s.loaded = true
	s.tok = &t
	s.gen++
	s.mu.Unlock()
	err := s.persist(t)
	s.notify(true)
	return err
}

// Token returns an access token that is valid now and for the refresh
// margin, refreshing it first if it is not. It returns [ErrSignedOut] when
// there is none to refresh or the server refused to (the stored tokens are
// then deleted). It blocks while the secure storage asks the user to unlock
// it and while the server answers; call it from a goroutine (the transport
// of [Session.Client] does).
func (s *Session) Token(ctx context.Context) (Token, error) {
	s.mu.Lock()
	if err := s.loadLocked(); err != nil {
		s.mu.Unlock()
		return Token{}, err
	}
	t := s.tok
	if t == nil {
		s.mu.Unlock()
		return Token{}, ErrSignedOut
	}
	if t.Valid(s.cfg.margin()) {
		out := *t
		s.mu.Unlock()
		return out, nil
	}
	if t.RefreshToken == "" {
		if t.expired() {
			s.dropExpiredLocked()
			s.mu.Unlock()
			return Token{}, fmt.Errorf("%w: the access token expired and there is no refresh token", ErrSignedOut)
		}
		out := *t
		s.mu.Unlock()
		return out, nil
	}
	f := s.flight
	if f == nil {
		f = &flight{done: make(chan struct{})}
		s.flight = f
		go s.refresh(f, *t, s.gen)
	}
	old := *t
	s.mu.Unlock()

	select {
	case <-f.done:
	case <-ctx.Done():
		return Token{}, ctx.Err()
	}
	if f.err != nil && !errors.Is(f.err, ErrSignedOut) && old.Valid(0) {
		return old, nil // the server is out of reach, and the token still works
	}
	return f.tok, f.err
}

// dropExpiredLocked signs out a session whose token expired with no way
// to renew it.
func (s *Session) dropExpiredLocked() {
	s.tok = nil
	s.gen++
	go func() {
		_ = desktop.DeleteSecret(s.cfg.secretName())
		s.notify(false)
	}()
}

// refresh runs one refresh and wakes the callers waiting for it. gen is
// the account's generation it started under.
func (s *Session) refresh(f *flight, old Token, gen int) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	nt, err := s.tokenRequest(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old.RefreshToken}})

	var oe *Error
	grantGone := errors.As(err, &oe) && oe.Code == "invalid_grant"
	if err == nil {
		if nt.RefreshToken == "" {
			nt.RefreshToken = old.RefreshToken // not rotated
		}
		if nt.Scope == "" {
			nt.Scope = old.Scope
		}
	}

	s.mu.Lock()
	changed := s.gen != gen
	switch {
	case changed: // signed in again or out meanwhile: the result is stale
		if s.tok != nil {
			f.tok = *s.tok
		} else {
			f.err = ErrSignedOut
		}
	case err == nil:
		s.tok = &nt
		f.tok = nt
	case grantGone:
		s.tok = nil
		s.gen++
		f.err = fmt.Errorf("%w: the server refused the refresh token (%v)", ErrSignedOut, err)
	default:
		f.err = fmt.Errorf("oauth: refreshing the token: %w", err)
	}
	s.flight = nil
	s.mu.Unlock()

	// A rotated refresh token is stored before the callers continue: a
	// crash after the server rotated it must not lose the new one. A
	// failure to store it leaves the tokens in memory; the next start
	// would fail with invalid_grant and sign in again.
	if !changed {
		switch {
		case err == nil:
			_ = s.persist(nt)
		case grantGone:
			_ = desktop.DeleteSecret(s.cfg.secretName())
			s.notify(false)
		}
	}
	close(f.done)
}

// SignOut forgets the account: the stored tokens are deleted and, when
// Config.RevokeURL is set, revoked at the server (RFC 7009). The local sign
// out always happens; the error reports a deletion that failed or a
// revocation the server did not accept (the tokens then live until they
// expire). It is fine to call it when signed out.
func (s *Session) SignOut(ctx context.Context) error {
	s.mu.Lock()
	loadErr := s.loadLocked()
	t := s.tok
	s.tok = nil
	s.gen++
	s.mu.Unlock()

	var errs []error
	if loadErr != nil {
		errs = append(errs, loadErr)
	}
	if derr := desktop.DeleteSecret(s.cfg.secretName()); derr != nil && !errors.Is(derr, desktop.ErrUnsupported) {
		errs = append(errs, fmt.Errorf("oauth: deleting the stored tokens: %w", derr))
	}
	s.notify(false)
	if t != nil && s.cfg.RevokeURL != "" {
		// Revoking the refresh token also ends the access tokens of the
		// grant (RFC 7009 §2.1); the access token is revoked too, for
		// servers that keep them apart.
		if t.RefreshToken != "" {
			if rerr := s.revoke(ctx, t.RefreshToken, "refresh_token"); rerr != nil {
				errs = append(errs, rerr)
			}
		}
		if t.AccessToken != "" {
			if rerr := s.revoke(ctx, t.AccessToken, "access_token"); rerr != nil && t.RefreshToken == "" {
				errs = append(errs, rerr)
			}
		}
	}
	return errors.Join(errs...)
}

// revoke sends one revocation request (RFC 7009 §2.1).
func (s *Session) revoke(ctx context.Context, token, hint string) error {
	form := url.Values{"token": {token}, "token_type_hint": {hint}, "client_id": {s.cfg.ClientID}}
	if s.cfg.ClientSecret != "" && s.cfg.SecretInBody {
		form.Set("client_secret", s.cfg.ClientSecret)
	}
	if _, err := s.post(ctx, s.cfg.RevokeURL, form); err != nil {
		return fmt.Errorf("oauth: revoking the %s: %w", hint, err)
	}
	return nil
}

// watch calls f, from any goroutine, whenever the session signs in (true)
// or out (false), until stop is called.
func (s *Session) watch(f func(signedIn bool)) (stop func()) {
	s.mu.Lock()
	s.listeners = append(s.listeners, f)
	i := len(s.listeners) - 1
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.listeners[i] = nil
		s.mu.Unlock()
	}
}

func (s *Session) notify(signedIn bool) {
	s.mu.Lock()
	ls := append([](func(bool)){}, s.listeners...)
	s.mu.Unlock()
	for _, f := range ls {
		if f != nil {
			f(signedIn)
		}
	}
}

// Client returns an HTTP client that sends the access token as a bearer
// token (RFC 6750) in every request, refreshing it when it is about to
// expire. A request fails with [ErrSignedOut] when there is no account.
// ctx bounds the client: after it is done every request fails with its
// error.
//
// The token goes only to https URLs (http to a loopback host is accepted,
// for tests), and a redirect to another host does not carry it. Use the
// client for the server's API only.
func (s *Session) Client(ctx context.Context) *http.Client {
	base := http.DefaultTransport
	if hc := s.cfg.HTTPClient; hc != nil && hc.Transport != nil {
		base = hc.Transport
	}
	return &http.Client{Transport: &transport{s: s, ctx: ctx, base: base}}
}

type transport struct {
	s    *Session
	ctx  context.Context
	base http.RoundTripper
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	closeBody := func() {
		if req.Body != nil {
			req.Body.Close()
		}
	}
	if err := t.ctx.Err(); err != nil {
		closeBody()
		return nil, err
	}
	if req.Response != nil && !sameOrigin(req.Response.Request.URL, req.URL) {
		return t.base.RoundTrip(req) // a redirect elsewhere: no token
	}
	if req.URL.Scheme != "https" && !isLoopback(req.URL.Hostname()) {
		closeBody()
		return nil, fmt.Errorf("oauth: refusing to send the access token to %s over plain http; use an https URL", req.URL.Redacted())
	}
	tok, err := t.s.Token(req.Context())
	if err != nil {
		closeBody()
		return nil, err
	}
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	return t.base.RoundTrip(r)
}

// sameOrigin reports whether b is a redirect that stays on a's host and
// does not downgrade it.
func sameOrigin(a, b *url.URL) bool {
	if a.Hostname() != b.Hostname() || a.Port() != b.Port() {
		return false
	}
	return a.Scheme == b.Scheme || (a.Scheme == "http" && b.Scheme == "https")
}
