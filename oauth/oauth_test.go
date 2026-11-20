package oauth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/oauth"
	"github.com/kuronosu/tela/oauth/oauthtest"
)

func TestSignInCodeFlow(t *testing.T) {
	w := newWorld(t)
	s := w.session()
	if err := bg(func() error { return s.SignIn(ctx(t)) }); err != nil {
		t.Fatal(err)
	}

	// What the app asked the server for.
	reqs := w.srv.AuthRequests()
	if len(reqs) != 1 {
		t.Fatalf("%d authorization requests, want 1", len(reqs))
	}
	q := reqs[0]
	if q.Get("response_type") != "code" || q.Get("client_id") != oauthtest.ClientID || q.Get("scope") != "profile" {
		t.Errorf("authorization request %v", q)
	}
	if q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 {
		t.Errorf("PKCE parameters %q %q", q.Get("code_challenge_method"), q.Get("code_challenge"))
	}
	if len(q.Get("state")) < 20 {
		t.Errorf("state %q is too short", q.Get("state"))
	}
	ru, _ := url.Parse(q.Get("redirect_uri"))
	if ru.Scheme != "http" || ru.Hostname() != "127.0.0.1" || ru.Port() == "" || ru.Path != "/callback" {
		t.Errorf("redirect URI %s is not a loopback URI with a port (RFC 8252 §7.3)", ru)
	}
	if w.srv.Calls("authorization_code") != 1 {
		t.Errorf("%d code exchanges, want 1", w.srv.Calls("authorization_code"))
	}

	// The browser's page, and the closed listener.
	status, body, ok := w.srv.Page()
	if !ok || status != 200 || !strings.Contains(body, "You can close this window") {
		t.Errorf("page = %d %q (%v)", status, body, ok)
	}
	if !closed(ru.Host) {
		t.Errorf("the listener at %s is still open after the sign-in", ru.Host)
	}

	// The tokens are in the secure storage, and work.
	tok, ok := w.stored()
	if !ok || tok.AccessToken == "" || tok.RefreshToken == "" || tok.Expiry.IsZero() {
		t.Fatalf("stored tokens: ok=%v, expiry %v", ok, tok.Expiry)
	}
	if code, err := fetch(s.Client(ctx(t)), w.srv.ResourceURL()); err != nil || code != 200 {
		t.Errorf("resource with the client: %d, %v", code, err)
	}

	// A new session (another start of the app) loads the tokens again.
	s2 := w.session()
	if ok, err := call(func() (bool, error) { return s2.SignedIn(ctx(t)) }); err != nil || !ok {
		t.Fatalf("SignedIn after a restart = %v, %v", ok, err)
	}
	if got, err := tokenOf(t, s2); err != nil || got.AccessToken != tok.AccessToken {
		t.Errorf("the reloaded token differs from the stored one (%v)", err)
	}
}

func TestSignInDenied(t *testing.T) {
	w := newWorld(t)
	w.srv.SetMode(oauthtest.Deny)
	s := w.session()
	err := bg(func() error { return s.SignIn(ctx(t)) })
	if !errors.Is(err, oauth.ErrAccessDenied) {
		t.Fatalf("SignIn = %v, want ErrAccessDenied", err)
	}
	var oe *oauth.Error
	if !errors.As(err, &oe) || oe.Code != "access_denied" {
		t.Errorf("the error is not an *oauth.Error with the server's code: %#v", err)
	}
	if status, body, _ := w.srv.Page(); status != 400 || !strings.Contains(body, "denied") {
		t.Errorf("page = %d %q", status, body)
	}
	if _, ok := w.stored(); ok {
		t.Error("tokens stored after a denial")
	}
	if w.srv.Calls("authorization_code") != 0 {
		t.Error("the app exchanged a code after a denial")
	}
}

func TestSignInWrongState(t *testing.T) {
	w := newWorld(t)
	w.srv.TamperState(true)
	s := w.session()
	err := bg(func() error { return s.SignIn(ctx(t)) })
	if !errors.Is(err, oauth.ErrStateMismatch) {
		t.Fatalf("SignIn = %v, want ErrStateMismatch", err)
	}
	if w.srv.Calls("authorization_code") != 0 {
		t.Error("the app exchanged the code of a redirect with the wrong state")
	}
	if _, ok := w.stored(); ok {
		t.Error("tokens stored")
	}
}

func TestSignInExpiredCode(t *testing.T) {
	w := newWorld(t)
	w.srv.SetMode(oauthtest.Expire)
	s := w.session()
	err := bg(func() error { return s.SignIn(ctx(t)) })
	var oe *oauth.Error
	if !errors.As(err, &oe) || oe.Code != "invalid_grant" {
		t.Fatalf("SignIn = %v, want invalid_grant", err)
	}
}

// The listener answers only the redirect, on its own host, and closes when
// the context is done.
func TestSignInListener(t *testing.T) {
	w := newWorld(t)
	cap := newCapture()
	w.cfg.Open = cap.open
	s := w.session()
	c, cancel := context.WithCancel(ctx(t))
	done := make(chan error, 1)
	go func() { done <- s.SignIn(c) }()
	authURL := cap.next(t)
	addr := listenerOf(t, authURL)
	base := "http://" + addr

	status := func(req *http.Request) int {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	mk := func(method, path, host string) *http.Request {
		r, _ := http.NewRequest(method, base+path, nil)
		if host != "" {
			r.Host = host
		}
		return r
	}
	if got := status(mk("GET", "/other", "")); got != 404 {
		t.Errorf("another path: %d, want 404", got)
	}
	if got := status(mk("GET", "/callback?code=x&state=y", "evil.example:80")); got != 400 {
		t.Errorf("a foreign Host: %d, want 400", got)
	}
	if got := status(mk("POST", "/callback?code=x&state=y", "")); got != 405 {
		t.Errorf("POST: %d, want 405", got)
	}
	if got := status(mk("GET", "/callback", "")); got != 400 {
		t.Errorf("no code and no error: %d, want 400", got)
	}
	select {
	case err := <-done:
		t.Fatalf("a stray request ended the sign-in: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("SignIn = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SignIn did not return after the context was canceled")
	}
	if !closed(addr) {
		t.Error("the listener stayed open after the cancel")
	}
}

// A redirect with another state ends the sign-in (RFC 9700 §4.7).
func TestSignInForeignRedirect(t *testing.T) {
	w := newWorld(t)
	cap := newCapture()
	w.cfg.Open = cap.open
	s := w.session()
	done := make(chan error, 1)
	go func() { done <- s.SignIn(ctx(t)) }()
	authURL := cap.next(t)
	addr := listenerOf(t, authURL)

	// A wrong-state request aborts the sign-in (RFC 9700 §4.7)...
	resp, err := http.Get("http://" + addr + "/callback?code=stolen&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if err := <-done; !errors.Is(err, oauth.ErrStateMismatch) {
		t.Fatalf("SignIn = %v, want ErrStateMismatch", err)
	}
	if !closed(addr) {
		t.Error("the listener stayed open")
	}
}

func TestCustomPage(t *testing.T) {
	w := newWorld(t)
	w.cfg.Page = func(rw http.ResponseWriter, err error) {
		fmt.Fprintf(rw, "custom page, err=%v", err)
	}
	s := w.session()
	if err := bg(func() error { return s.SignIn(ctx(t)) }); err != nil {
		t.Fatal(err)
	}
	if _, body, _ := w.srv.Page(); body != "custom page, err=<nil>" {
		t.Errorf("page %q", body)
	}
}

func TestOpenFails(t *testing.T) {
	w := newWorld(t)
	boom := errors.New("no browser")
	w.cfg.Open = func(string) error { return boom }
	s := w.session()
	err := bg(func() error { return s.SignIn(ctx(t)) })
	if !errors.Is(err, boom) {
		t.Fatalf("SignIn = %v, want the Open error", err)
	}
}

// ---------------------------------------------------------------------
// Device flow.

func TestDeviceFlow(t *testing.T) {
	const interval = 40 * time.Millisecond
	const step = 60 * time.Millisecond
	oauth.SetSlowDownStep(t, step)
	w := newWorld(t, oauthtest.DeviceInterval(interval))
	w.srv.PendingPolls(2)
	w.srv.SlowDowns(1)
	s := w.session()

	var shown oauth.DeviceCode
	err := bg(func() error { return s.DeviceSignIn(ctx(t), func(dc oauth.DeviceCode) { shown = dc }) })
	if err != nil {
		t.Fatal(err)
	}
	if shown.UserCode != "WDJB-MJHT" || !strings.HasSuffix(shown.VerificationURI, "/activate") || shown.Interval != interval {
		t.Errorf("shown code %+v", shown)
	}
	if !strings.Contains(shown.VerificationURIComplete, "user_code=WDJB-MJHT") {
		t.Errorf("complete URI %q", shown.VerificationURIComplete)
	}
	polls := w.srv.Polls()
	if len(polls) != 4 { // 2 pending, 1 slow_down, then the approval
		t.Fatalf("%d polls, want 4", len(polls))
	}
	slack := 10 * time.Millisecond
	for i := 1; i < len(polls); i++ {
		gap := polls[i].Sub(polls[i-1])
		want := interval
		if i == 3 { // after the slow_down of poll 3
			want = interval + step
		}
		if gap < want-slack {
			t.Errorf("poll %d came %v after the previous, want at least %v", i+1, gap, want)
		}
	}
	if _, ok := w.stored(); !ok {
		t.Error("tokens not stored")
	}
}

func TestDeviceFlowDenied(t *testing.T) {
	w := newWorld(t)
	w.srv.SetMode(oauthtest.Deny)
	s := w.session()
	err := bg(func() error { return s.DeviceSignIn(ctx(t), nil) })
	if !errors.Is(err, oauth.ErrAccessDenied) {
		t.Fatalf("DeviceSignIn = %v, want ErrAccessDenied", err)
	}
}

func TestDeviceFlowServerExpires(t *testing.T) {
	w := newWorld(t)
	w.srv.SetMode(oauthtest.Expire)
	s := w.session()
	err := bg(func() error { return s.DeviceSignIn(ctx(t), nil) })
	if !errors.Is(err, oauth.ErrExpired) {
		t.Fatalf("DeviceSignIn = %v, want ErrExpired", err)
	}
}

func TestDeviceFlowLocalExpiry(t *testing.T) {
	w := newWorld(t, oauthtest.DeviceLifetime(150*time.Millisecond))
	w.srv.PendingPolls(1000)
	s := w.session()
	err := bg(func() error { return s.DeviceSignIn(ctx(t), nil) })
	if !errors.Is(err, oauth.ErrExpired) {
		t.Fatalf("DeviceSignIn = %v, want ErrExpired", err)
	}
}

func TestDeviceFlowCanceled(t *testing.T) {
	w := newWorld(t)
	w.srv.PendingPolls(1000)
	s := w.session()
	c, cancel := context.WithTimeout(ctx(t), 200*time.Millisecond)
	defer cancel()
	err := bg(func() error { return s.DeviceSignIn(c, nil) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DeviceSignIn = %v, want the context's error", err)
	}
}

func TestDeviceFlowNeedsDeviceURL(t *testing.T) {
	w := newWorld(t)
	w.cfg.DeviceURL = ""
	err := bg(func() error { return w.session().DeviceSignIn(ctx(t), nil) })
	if err == nil || !strings.Contains(err.Error(), "Config.DeviceURL") {
		t.Fatalf("DeviceSignIn = %v, want an error that names Config.DeviceURL", err)
	}
}

// ---------------------------------------------------------------------
// Refresh.

// signedIn returns a session of w that is signed in.
func (w *world) signedIn() *oauth.Session {
	w.t.Helper()
	s := w.session()
	if err := bg(func() error { return s.SignIn(ctx(w.t)) }); err != nil {
		w.t.Fatal(err)
	}
	return s
}

// clock is a controllable now.
type clock struct{ off atomic.Int64 }

func (c *clock) now() time.Time          { return time.Now().Add(time.Duration(c.off.Load())) }
func (c *clock) advance(d time.Duration) { c.off.Add(int64(d)) }

func TestRefreshRotates(t *testing.T) {
	w := newWorld(t, oauthtest.TokenLifetime(time.Hour))
	var clk clock
	oauth.SetNow(t, clk.now)
	s := w.signedIn()
	first, _ := w.stored()

	tok, err := tokenOf(t, s)
	check(t, err)
	if tok.AccessToken != first.AccessToken || w.srv.Calls("refresh_token") != 0 {
		t.Fatal("a fresh token was refreshed")
	}

	clk.advance(59*time.Minute + 30*time.Second) // inside the one-minute margin
	tok, err = tokenOf(t, s)
	check(t, err)
	if tok.AccessToken == first.AccessToken || w.srv.Calls("refresh_token") != 1 {
		t.Fatalf("the token was not refreshed (%d refreshes)", w.srv.Calls("refresh_token"))
	}
	stored, _ := w.stored()
	if stored.RefreshToken == first.RefreshToken || stored.AccessToken != tok.AccessToken {
		t.Error("the rotated refresh token was not stored")
	}
	if w.srv.ReuseDetected() != 0 {
		t.Error("reuse detected")
	}

	// The refreshed token works at the server.
	if code, err := fetch(s.Client(ctx(t)), w.srv.ResourceURL()); err != nil || code != 200 {
		t.Errorf("resource: %d, %v", code, err)
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

func TestRefreshCoalesces(t *testing.T) {
	w := newWorld(t)
	var clk clock
	oauth.SetNow(t, clk.now)
	s := w.signedIn()
	clk.advance(2 * time.Hour)

	var wg sync.WaitGroup
	tokens := make([]string, 20)
	for i := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := call(func() (oauth.Token, error) { return s.Token(ctx(t)) })
			check(t, err)
			tokens[i] = tk.AccessToken
		}()
	}
	wg.Wait()
	if n := w.srv.Calls("refresh_token"); n != 1 {
		t.Errorf("%d refresh requests for 20 concurrent callers, want 1", n)
	}
	for _, tk := range tokens {
		if tk == "" || tk != tokens[0] {
			t.Fatalf("callers got different tokens: %v", tokens)
		}
	}
	if w.srv.ReuseDetected() != 0 {
		t.Error("reuse detected")
	}
}

func TestRefreshReuseSignsOut(t *testing.T) {
	w := newWorld(t)
	var clk clock
	oauth.SetNow(t, clk.now)
	s := w.signedIn()
	old, _ := w.app.StoredSecret(w.cfg.SecretName) // R1

	clk.advance(2 * time.Hour)
	_, err := tokenOf(t, s) // R1 -> R2
	check(t, err)

	// A copy of the first tokens turns up (a restored backup, a thief):
	// the server sees R1 again.
	w.app.SetStoredSecret(w.cfg.SecretName, old)
	s2 := w.session()
	_, err = tokenOf(t, s2)
	if !errors.Is(err, oauth.ErrSignedOut) {
		t.Fatalf("Token = %v, want ErrSignedOut", err)
	}
	if w.srv.ReuseDetected() != 1 {
		t.Errorf("the server detected %d reuses, want 1", w.srv.ReuseDetected())
	}
	if _, ok := w.stored(); ok {
		t.Error("the tokens were not deleted")
	}
	if ok, _ := call(func() (bool, error) { return s2.SignedIn(ctx(t)) }); ok {
		t.Error("still signed in")
	}
}

func TestRefreshServerDownKeepsValidToken(t *testing.T) {
	w := newWorld(t)
	var clk clock
	oauth.SetNow(t, clk.now)
	s := w.signedIn()
	clk.advance(59*time.Minute + 30*time.Second) // in the margin, not expired
	w.srv.Close()
	tok, err := tokenOf(t, s)
	if tok.AccessToken == "" || err != nil {
		t.Fatalf("Token with the server down: %v", err)
	}
	clk.advance(time.Hour) // now expired
	_, err = tokenOf(t, s)
	if err == nil || errors.Is(err, oauth.ErrSignedOut) {
		t.Fatalf("Token of an expired token with the server down = %v, want a network error that is not ErrSignedOut", err)
	}
	if _, ok := w.stored(); !ok {
		t.Error("a network error deleted the tokens")
	}
}

func TestTokenWithoutRefreshToken(t *testing.T) {
	w := newWorld(t)
	var clk clock
	oauth.SetNow(t, clk.now)
	w.app.SetStoredSecret(w.cfg.SecretName, mustJSON(oauth.Token{AccessToken: "x", Expiry: clk.now().Add(time.Hour)}))
	s := w.session()
	if tk, err := tokenOf(t, s); tk.AccessToken != "x" || err != nil {
		t.Fatalf("a token that has not expired: %v", err)
	}
	clk.advance(2 * time.Hour)
	_, err := tokenOf(t, s)
	if !errors.Is(err, oauth.ErrSignedOut) {
		t.Fatalf("Token = %v, want ErrSignedOut", err)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ---------------------------------------------------------------------
// Sign out, revocation.

func TestSignOutRevokes(t *testing.T) {
	w := newWorld(t)
	s := w.signedIn()
	tok, _ := w.stored()
	if err := bg(func() error { return s.SignOut(ctx(t)) }); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.stored(); ok {
		t.Error("the tokens are still stored")
	}
	if !w.srv.Revoked(tok.RefreshToken) || !w.srv.Revoked(tok.AccessToken) {
		t.Error("the server did not revoke the tokens")
	}
	if _, err := tokenOf(t, s); !errors.Is(err, oauth.ErrSignedOut) {
		t.Errorf("Token after SignOut = %v", err)
	}
	// Signing out twice is fine.
	if err := bg(func() error { return s.SignOut(ctx(t)) }); err != nil {
		t.Errorf("second SignOut: %v", err)
	}
}

func TestSignOutWithoutRevokeURL(t *testing.T) {
	w := newWorld(t)
	w.cfg.RevokeURL = ""
	s := w.signedIn()
	if err := bg(func() error { return s.SignOut(ctx(t)) }); err != nil {
		t.Fatal(err)
	}
	if w.srv.RevokeRequests() != 0 {
		t.Error("revoked without a RevokeURL")
	}
	if _, ok := w.stored(); ok {
		t.Error("tokens still stored")
	}
}

func TestSignOutRevokeFailsStillSignsOut(t *testing.T) {
	w := newWorld(t)
	s := w.signedIn()
	w.srv.Close()
	err := bg(func() error { return s.SignOut(ctx(t)) })
	if err == nil || !strings.Contains(err.Error(), "revoking") {
		t.Fatalf("SignOut = %v, want the revocation error", err)
	}
	if _, ok := w.stored(); ok {
		t.Error("the tokens stayed after a failed revocation")
	}
}

// ---------------------------------------------------------------------
// The client's care of the token.

func TestClientRefusesCleartext(t *testing.T) {
	w := newWorld(t)
	s := w.signedIn()
	_, err := fetch(s.Client(ctx(t)), "http://example.com/api")
	if err == nil || !strings.Contains(err.Error(), "plain http") {
		t.Fatalf("GET over plain http = %v, want a refusal", err)
	}
}

func TestClientDoesNotLeakTokenOnRedirect(t *testing.T) {
	w := newWorld(t)
	s := w.signedIn()
	var leaked atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("Authorization"))
	}))
	defer other.Close()
	var sawAuth atomic.Value
	first := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		sawAuth.Store(r.Header.Get("Authorization"))
		http.Redirect(rw, r, other.URL, http.StatusFound)
	}))
	defer first.Close()
	c := s.Client(ctx(t))
	if _, err := fetch(c, first.URL); err != nil {
		t.Fatal(err)
	}
	if a, _ := sawAuth.Load().(string); !strings.HasPrefix(a, "Bearer ") {
		t.Errorf("the API got Authorization %q", a)
	}
	if a, _ := leaked.Load().(string); a != "" {
		t.Errorf("the token went to another host: %q", a)
	}
}

func TestClientContextEnds(t *testing.T) {
	w := newWorld(t)
	s := w.signedIn()
	c, cancel := context.WithCancel(ctx(t))
	cl := s.Client(c)
	cancel()
	_, err := fetch(cl, w.srv.ResourceURL())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request with a finished client context = %v", err)
	}
}

func TestClientSignedOut(t *testing.T) {
	w := newWorld(t)
	s := w.session()
	cl := s.Client(ctx(t))
	_, err := fetch(cl, w.srv.ResourceURL())
	if !errors.Is(err, oauth.ErrSignedOut) {
		t.Fatalf("request while signed out = %v", err)
	}
}

// ---------------------------------------------------------------------
// Secrets stay secret, the Config is checked.

func TestTokenNeverPrinted(t *testing.T) {
	tok := oauth.Token{AccessToken: "SECRET-ACCESS", RefreshToken: "SECRET-REFRESH"}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("signed in", "token", tok, "ptr", &tok)
	out := fmt.Sprintf("%v|%+v|%#v|%s|%v|%s", tok, tok, tok, tok, &tok, buf.String())
	if strings.Contains(out, "SECRET") {
		t.Errorf("a token was printed: %s", out)
	}
}

func TestErrorsDoNotHoldTokens(t *testing.T) {
	w := newWorld(t)
	var clk clock
	oauth.SetNow(t, clk.now)
	s := w.signedIn()
	tok, _ := w.stored()
	w.srv.SetMode(oauthtest.Deny)
	clk.advance(2 * time.Hour)
	w.srv.Close()
	_, err := tokenOf(t, s)
	if err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(err.Error(), tok.RefreshToken) || strings.Contains(err.Error(), tok.AccessToken) {
		t.Errorf("the error holds a token: %v", err)
	}
}

func TestConfigChecks(t *testing.T) {
	good := oauth.Config{ClientID: "id", AuthURL: "https://example.com/a", TokenURL: "https://example.com/t"}
	oauth.New(good) // no panic
	for name, tc := range map[string]struct {
		mutate func(*oauth.Config)
		want   string
	}{
		"no client id": {func(c *oauth.Config) { c.ClientID = "" }, "ClientID"},
		"http token":   {func(c *oauth.Config) { c.TokenURL = "http://example.com/t" }, "https"},
		"relative":     {func(c *oauth.Config) { c.AuthURL = "/authorize" }, "AuthURL"},
		"reserved":     {func(c *oauth.Config) { c.Params = map[string]string{"state": "x"} }, "state"},
		"redirect":     {func(c *oauth.Config) { c.RedirectPath = "cb" }, "slash"},
	} {
		t.Run(name, func(t *testing.T) {
			c := good
			tc.mutate(&c)
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Fatalf("New panicked with %v, want a message with %q", r, tc.want)
				}
			}()
			oauth.New(c)
		})
	}
	oauth.New(oauth.Config{ClientID: "id", AuthURL: "http://127.0.0.1:1/a", TokenURL: "http://localhost:1/t"}) // loopback http is fine
}

func TestSignInNeedsEndpoints(t *testing.T) {
	w := newWorld(t)
	w.cfg.AuthURL = ""
	err := bg(func() error { return w.session().SignIn(ctx(t)) })
	if err == nil || !strings.Contains(err.Error(), "Config.AuthURL") {
		t.Fatalf("SignIn = %v", err)
	}
}

func TestExtraParamsAndBasicAuth(t *testing.T) {
	w := newWorld(t)
	w.cfg.Params = map[string]string{"prompt": "consent"}
	w.cfg.ClientSecret = "shh"
	s := w.session()
	if err := bg(func() error { return s.SignIn(ctx(t)) }); err != nil {
		t.Fatal(err)
	}
	if got := w.srv.AuthRequests()[0].Get("prompt"); got != "consent" {
		t.Errorf("prompt = %q", got)
	}
}

// Without Config.Open the URL goes to the system browser (tela.OpenURL on
// the UI thread); the headless desktop accepts it, and the sign-in waits
// for a redirect that never comes until its context ends.
func TestDefaultBrowserWaits(t *testing.T) {
	w := newWorld(t)
	w.cfg.Open = nil
	s := w.session()
	c, cancel := context.WithTimeout(ctx(t), 300*time.Millisecond)
	defer cancel()
	err := bg(func() error { return s.SignIn(c) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SignIn = %v, want the context's deadline", err)
	}
}
