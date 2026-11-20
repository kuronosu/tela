// Package oauthtest is a fake OAuth 2.0 server for the tests of an app that
// uses package oauth, and for telatest (ADR-298). It is to oauth what
// net/http/httptest is to net/http: start it, point the app's oauth.Config
// at it, and let it approve, deny or expire.
//
//	srv := oauthtest.NewServer()
//	defer srv.Close()
//	cfg := srv.Config() // endpoints, a client ID, and srv.Browser as Open
//	acct := oauth.New(cfg)
//	err := acct.SignIn(ctx) // the fake browser follows the authorization URL
//
// It implements the authorization endpoint (authorization code with PKCE
// S256, redirects to a loopback URI), the token endpoint (authorization
// code, refresh token with rotation and reuse detection, device code), the
// device authorization endpoint (RFC 8628), the revocation endpoint (RFC
// 7009), and a protected resource that answers 200 to a valid access token.
// Like a real server it is strict: a missing or wrong code verifier, a
// code used twice, a redirect URI that is not loopback http, or a refresh
// token used a second time are errors.
//
// It imports only package oauth and the standard library.
package oauthtest

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela/oauth"
)

// Mode is how the server answers a sign-in.
type Mode int

const (
	// Approve (the default): the user approves at once.
	Approve Mode = iota
	// Deny: the user says no (error=access_denied).
	Deny
	// Expire: the authorization code, or the device code, has expired by
	// the time the app uses it (invalid_grant, expired_token).
	Expire
)

// ClientID is the client ID of Config.
const ClientID = "oauthtest-client"

// Server is a running fake server.
type Server struct {
	// URL is the base URL, "http://127.0.0.1:port".
	URL string

	ts *httptest.Server

	mu           sync.Mutex
	mode         Mode
	lifetime     time.Duration
	interval     time.Duration
	deviceTTL    time.Duration
	pending      int // device polls still answered authorization_pending
	slowDowns    int // device polls still answered slow_down
	tamperState  bool
	codes        map[string]*authCode
	devices      map[string]*deviceGrant
	access       map[string]*accessToken
	refresh      map[string]*refreshToken
	revokedGrant map[string]bool
	calls        map[string]int
	authReqs     []url.Values
	polls        []time.Time
	reuses       int
	revokes      []string
	pages        chan page
	seq          int
}

type authCode struct {
	challenge, redirect string
	expires             time.Time
	used                bool
	grant               string
}

type deviceGrant struct {
	expires time.Time
	grant   string
}

type accessToken struct {
	expires time.Time
	grant   string
	revoked bool
}

type refreshToken struct {
	grant string
	used  bool
}

type page struct {
	status int
	body   string
}

// Option configures NewServer.
type Option func(*Server)

// TokenLifetime sets how long access tokens live (default one hour). A
// lifetime under a second is sent as a fraction (expires_in: 0.3), which
// package oauth accepts.
func TokenLifetime(d time.Duration) Option { return func(s *Server) { s.lifetime = d } }

// DeviceInterval sets the polling interval of the device flow (default
// 50 ms, to keep tests fast; a real server says 5).
func DeviceInterval(d time.Duration) Option { return func(s *Server) { s.interval = d } }

// DeviceLifetime sets how long a device code is valid (default 10 s).
func DeviceLifetime(d time.Duration) Option { return func(s *Server) { s.deviceTTL = d } }

// NewServer starts a server; Close it when the test is done.
func NewServer(opts ...Option) *Server {
	s := &Server{
		lifetime:     time.Hour,
		interval:     50 * time.Millisecond,
		deviceTTL:    10 * time.Second,
		codes:        map[string]*authCode{},
		devices:      map[string]*deviceGrant{},
		access:       map[string]*accessToken{},
		refresh:      map[string]*refreshToken{},
		revokedGrant: map[string]bool{},
		calls:        map[string]int{},
		pages:        make(chan page, 32),
	}
	for _, o := range opts {
		o(s)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", s.handleAuthorize)
	mux.HandleFunc("/token", s.handleToken)
	mux.HandleFunc("/device", s.handleDevice)
	mux.HandleFunc("/revoke", s.handleRevoke)
	mux.HandleFunc("/resource", s.handleResource)
	s.ts = httptest.NewServer(mux)
	s.URL = s.ts.URL
	return s
}

// Close shuts the server down.
func (s *Server) Close() { s.ts.Close() }

// Config returns the oauth.Config of a client of this server: its
// endpoints, ClientID, a scope, and Browser as Open. Change what a test
// needs.
func (s *Server) Config() oauth.Config {
	return oauth.Config{
		ClientID:   ClientID,
		AuthURL:    s.URL + "/authorize",
		TokenURL:   s.URL + "/token",
		DeviceURL:  s.URL + "/device",
		RevokeURL:  s.URL + "/revoke",
		Scopes:     []string{"profile"},
		SecretName: "oauthtest-token",
		Open:       s.Browser(),
	}
}

// ResourceURL is a protected resource: 200 with a valid access token as a
// bearer token, 401 otherwise.
func (s *Server) ResourceURL() string { return s.URL + "/resource" }

// SetMode changes how the server answers from now on.
func (s *Server) SetMode(m Mode) {
	s.mu.Lock()
	s.mode = m
	s.mu.Unlock()
}

// SetTokenLifetime changes the lifetime of the access tokens issued from
// now on.
func (s *Server) SetTokenLifetime(d time.Duration) {
	s.mu.Lock()
	s.lifetime = d
	s.mu.Unlock()
}

// PendingPolls makes the next n polls of a device code answer
// authorization_pending before the user approves.
func (s *Server) PendingPolls(n int) {
	s.mu.Lock()
	s.pending = n
	s.mu.Unlock()
}

// SlowDowns makes n polls of a device code, after the pending ones, answer
// slow_down.
func (s *Server) SlowDowns(n int) {
	s.mu.Lock()
	s.slowDowns = n
	s.mu.Unlock()
}

// TamperState makes the authorization endpoint redirect with a state that
// is not the one it received, as an attacker's redirect would.
func (s *Server) TamperState(on bool) {
	s.mu.Lock()
	s.tamperState = on
	s.mu.Unlock()
}

// Calls returns how many requests the token endpoint got for a grant type:
// "authorization_code", "refresh_token" or "urn:ietf:params:oauth:grant-type:device_code".
func (s *Server) Calls(grant string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[grant]
}

// AuthRequests returns the query of every authorization request, in order.
func (s *Server) AuthRequests() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.authReqs...)
}

// Polls returns when each poll of a device code arrived, to check that
// the client waited for the interval.
func (s *Server) Polls() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.polls...)
}

// Interval is the device polling interval the server announces.
func (s *Server) Interval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

// ReuseDetected returns how many times a refresh token was used a second
// time (each revoked its whole grant, RFC 9700 §4.14).
func (s *Server) ReuseDetected() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reuses
}

// Revoked reports whether token (an access or a refresh token) has been
// revoked, by the revocation endpoint or by reuse detection.
func (s *Server) Revoked(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.access[token]; ok {
		return a.revoked || s.revokedGrant[a.grant]
	}
	if r, ok := s.refresh[token]; ok {
		return s.revokedGrant[r.grant]
	}
	return false
}

// RevokeRequests returns how many tokens were sent to the revocation
// endpoint.
func (s *Server) RevokeRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.revokes)
}

// ExpireAccessTokens makes every access token issued so far expired.
func (s *Server) ExpireAccessTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.access {
		a.expires = time.Now().Add(-time.Second)
	}
}

// ---------------------------------------------------------------------

func (s *Server) newToken(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *Server) newGrant() string {
	s.seq++
	return fmt.Sprintf("grant-%d", s.seq)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// loopbackRedirect reports whether raw is an http URI of a loopback host,
// any port (RFC 8252 §7.3).
func loopbackRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.mu.Lock()
	s.authReqs = append(s.authReqs, q)
	mode, tamper := s.mode, s.tamperState
	s.mu.Unlock()

	redirect := q.Get("redirect_uri")
	if q.Get("client_id") != ClientID || !loopbackRedirect(redirect) {
		http.Error(w, "bad client_id or redirect_uri (must be http on a loopback host)", http.StatusBadRequest)
		return
	}
	back := func(vals url.Values) {
		state := q.Get("state")
		if tamper {
			state = "tampered-" + state
		}
		vals.Set("state", state)
		u, _ := url.Parse(redirect)
		rq := u.Query()
		for k, v := range vals {
			rq[k] = v
		}
		u.RawQuery = rq.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	}
	switch {
	case q.Get("response_type") != "code":
		back(url.Values{"error": {"unsupported_response_type"}})
		return
	case q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256":
		back(url.Values{"error": {"invalid_request"}, "error_description": {"PKCE with S256 is required"}})
		return
	case mode == Deny:
		back(url.Values{"error": {"access_denied"}, "error_description": {"the user denied the request"}})
		return
	}
	s.mu.Lock()
	code := s.newToken("code")
	ttl := time.Minute
	if mode == Expire {
		ttl = -time.Second
	}
	s.codes[code] = &authCode{challenge: q.Get("code_challenge"), redirect: redirect, expires: time.Now().Add(ttl), grant: s.newGrant()}
	s.mu.Unlock()
	back(url.Values{"code": {code}})
}

// clientID returns the client the request authenticates as.
func clientID(r *http.Request) string {
	if id, _, ok := r.BasicAuth(); ok {
		id, _ = url.QueryUnescape(id)
		return id
	}
	return r.PostForm.Get("client_id")
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.ParseForm() != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "POST a form")
		return
	}
	if clientID(r) != ClientID {
		oauthError(w, http.StatusUnauthorized, "invalid_client", "unknown client")
		return
	}
	grant := r.PostForm.Get("grant_type")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[grant]++
	switch grant {
	case "authorization_code":
		s.codeGrant(w, r)
	case "refresh_token":
		s.refreshGrant(w, r)
	case "urn:ietf:params:oauth:grant-type:device_code":
		s.deviceGrant(w, r)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", grant)
	}
}

// issue makes the tokens of a grant; the caller holds s.mu.
func (s *Server) issue(w http.ResponseWriter, grant string, withRefresh bool) {
	at := s.newToken("at")
	s.access[at] = &accessToken{expires: time.Now().Add(s.lifetime), grant: grant}
	resp := map[string]any{
		"access_token": at,
		"token_type":   "Bearer",
		"expires_in":   s.lifetime.Seconds(),
		"scope":        "profile",
	}
	if withRefresh {
		rt := s.newToken("rt")
		s.refresh[rt] = &refreshToken{grant: grant}
		resp["refresh_token"] = rt
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) codeGrant(w http.ResponseWriter, r *http.Request) {
	c := s.codes[r.PostForm.Get("code")]
	switch {
	case c == nil:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "unknown code")
		return
	case c.used:
		s.revokedGrant[c.grant] = true // RFC 6749 §4.1.2: a reused code revokes what it made
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code was already used")
		return
	}
	c.used = true
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	switch {
	case time.Now().After(c.expires):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code expired")
	case r.PostForm.Get("code_verifier") == "":
		oauthError(w, http.StatusBadRequest, "invalid_request", "code_verifier is required")
	case subtle.ConstantTimeCompare([]byte(got), []byte(c.challenge)) != 1:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
	case r.PostForm.Get("redirect_uri") != c.redirect:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the request's")
	default:
		s.issue(w, c.grant, true)
	}
}

func (s *Server) refreshGrant(w http.ResponseWriter, r *http.Request) {
	rt := s.refresh[r.PostForm.Get("refresh_token")]
	switch {
	case rt == nil:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "unknown refresh token")
	case s.revokedGrant[rt.grant]:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the grant was revoked")
	case rt.used:
		// A rotated token came back: someone copied it (RFC 9700 §4.14).
		s.reuses++
		s.revokedGrant[rt.grant] = true
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token reuse detected; the grant was revoked")
	default:
		rt.used = true
		s.issue(w, rt.grant, true)
	}
}

func (s *Server) deviceGrant(w http.ResponseWriter, r *http.Request) {
	d := s.devices[r.PostForm.Get("device_code")]
	if d == nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "unknown device code")
		return
	}
	s.polls = append(s.polls, time.Now())
	switch {
	case s.mode == Expire || time.Now().After(d.expires):
		oauthError(w, http.StatusBadRequest, "expired_token", "the device code expired")
	case s.mode == Deny:
		oauthError(w, http.StatusBadRequest, "access_denied", "the user denied the request")
	case s.pending > 0:
		s.pending--
		oauthError(w, http.StatusBadRequest, "authorization_pending", "the user has not approved yet")
	case s.slowDowns > 0:
		s.slowDowns--
		oauthError(w, http.StatusBadRequest, "slow_down", "poll less often")
	default:
		s.issue(w, d.grant, true)
	}
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.ParseForm() != nil || clientID(r) != ClientID {
		oauthError(w, http.StatusBadRequest, "invalid_client", "POST a form with the client_id")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dc := s.newToken("dc")
	s.devices[dc] = &deviceGrant{expires: time.Now().Add(s.deviceTTL), grant: s.newGrant()}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               dc,
		"user_code":                 "WDJB-MJHT",
		"verification_uri":          s.URL + "/activate",
		"verification_uri_complete": s.URL + "/activate?user_code=WDJB-MJHT",
		"expires_in":                s.deviceTTL.Seconds(),
		"interval":                  s.interval.Seconds(),
	})
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.ParseForm() != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "POST a form")
		return
	}
	tok := r.PostForm.Get("token")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokes = append(s.revokes, tok)
	// RFC 7009 §2.2: an unknown token is not an error.
	if a, ok := s.access[tok]; ok {
		a.revoked = true
	}
	if rt, ok := s.refresh[tok]; ok {
		s.revokedGrant[rt.grant] = true
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleResource(w http.ResponseWriter, r *http.Request) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	a := s.access[tok]
	valid := ok && a != nil && time.Now().Before(a.expires) && !a.revoked && !s.revokedGrant[a.grant]
	s.mu.Unlock()
	if !valid {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("hello from the resource server"))
}
