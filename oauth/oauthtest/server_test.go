package oauthtest_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/oauth/oauthtest"
)

const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk" // RFC 7636 appendix B

func challenge(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// noFollow is a client that shows the redirects instead of following them.
var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// authorize asks for a code with the challenge of verifier and returns the
// redirect's query.
func authorize(t *testing.T, srv *oauthtest.Server, redirect, chal, method string) (int, url.Values) {
	t.Helper()
	q := url.Values{"response_type": {"code"}, "client_id": {oauthtest.ClientID}, "redirect_uri": {redirect}, "state": {"st"}}
	if chal != "" {
		q.Set("code_challenge", chal)
		q.Set("code_challenge_method", method)
	}
	resp, err := noFollow.Get(srv.URL + "/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, loc.Query()
}

type tokenReply struct {
	status int
	Access string  `json:"access_token"`
	Refr   string  `json:"refresh_token"`
	Expire float64 `json:"expires_in"`
	Err    string  `json:"error"`
}

func token(t *testing.T, srv *oauthtest.Server, form url.Values) tokenReply {
	t.Helper()
	form.Set("client_id", oauthtest.ClientID)
	resp, err := http.PostForm(srv.URL+"/token", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := tokenReply{status: resp.StatusCode}
	_ = json.NewDecoder(resp.Body).Decode(&r)
	return r
}

func exchange(t *testing.T, srv *oauthtest.Server, code, ver string) tokenReply {
	return token(t, srv, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {ver}, "redirect_uri": {"http://127.0.0.1:5/cb"}})
}

func TestPKCEMismatch(t *testing.T) {
	srv := oauthtest.NewServer()
	defer srv.Close()
	status, q := authorize(t, srv, "http://127.0.0.1:5/cb", challenge(verifier), "S256")
	if status != http.StatusFound || q.Get("code") == "" || q.Get("state") != "st" {
		t.Fatalf("authorize: %d %v", status, q)
	}
	code := q.Get("code")
	if r := exchange(t, srv, code, "another-verifier-that-is-long-enough-to-pass"); r.status != 400 || r.Err != "invalid_grant" {
		t.Errorf("a wrong verifier: %d %q, want 400 invalid_grant", r.status, r.Err)
	}
	// The failed try burned the code.
	if r := exchange(t, srv, code, verifier); r.Err != "invalid_grant" {
		t.Errorf("the code worked after a failed try: %+v", r)
	}
}

func TestPKCEMatchAndCodeReuse(t *testing.T) {
	srv := oauthtest.NewServer()
	defer srv.Close()
	_, q := authorize(t, srv, "http://127.0.0.1:5/cb", challenge(verifier), "S256")
	first := exchange(t, srv, q.Get("code"), verifier)
	if first.status != 200 || first.Access == "" || first.Refr == "" {
		t.Fatalf("exchange: %+v", first)
	}
	// A second use revokes what the first made (RFC 6749 §4.1.2).
	if r := exchange(t, srv, q.Get("code"), verifier); r.Err != "invalid_grant" {
		t.Errorf("reused code: %+v", r)
	}
	if !srv.Revoked(first.Access) || !srv.Revoked(first.Refr) {
		t.Error("the tokens of a reused code stayed valid")
	}
}

func TestAuthorizeStrictness(t *testing.T) {
	srv := oauthtest.NewServer()
	defer srv.Close()
	if _, q := authorize(t, srv, "http://127.0.0.1:5/cb", "", ""); q.Get("error") != "invalid_request" {
		t.Errorf("no PKCE: %v", q)
	}
	if _, q := authorize(t, srv, "http://127.0.0.1:5/cb", verifier, "plain"); q.Get("error") != "invalid_request" {
		t.Errorf("plain PKCE: %v", q)
	}
	resp, err := noFollow.Get(srv.URL + "/authorize?response_type=code&client_id=" + oauthtest.ClientID + "&redirect_uri=" + url.QueryEscape("https://evil.example/cb") + "&code_challenge=x&code_challenge_method=S256")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("a redirect to another host: %d, want 400 (no redirect)", resp.StatusCode)
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	srv := oauthtest.NewServer()
	defer srv.Close()
	_, q := authorize(t, srv, "http://127.0.0.1:5/cb", challenge(verifier), "S256")
	first := exchange(t, srv, q.Get("code"), verifier)
	refresh := func(rt string) tokenReply {
		return token(t, srv, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rt}})
	}
	second := refresh(first.Refr)
	if second.status != 200 || second.Refr == "" || second.Refr == first.Refr || second.Access == first.Access {
		t.Fatalf("rotation: %+v", second)
	}
	// R1 again: the grant is revoked, including the newest tokens.
	if r := refresh(first.Refr); r.Err != "invalid_grant" {
		t.Errorf("reused refresh token: %+v", r)
	}
	if srv.ReuseDetected() != 1 || !srv.Revoked(second.Refr) || !srv.Revoked(second.Access) {
		t.Errorf("reuse detection: %d detected, newest revoked %v", srv.ReuseDetected(), srv.Revoked(second.Refr))
	}
	if r := refresh(second.Refr); r.Err != "invalid_grant" {
		t.Errorf("a refresh token of a revoked grant worked: %+v", r)
	}
}

func TestTokenLifetimeAndResource(t *testing.T) {
	srv := oauthtest.NewServer(oauthtest.TokenLifetime(300 * time.Millisecond))
	defer srv.Close()
	_, q := authorize(t, srv, "http://127.0.0.1:5/cb", challenge(verifier), "S256")
	tok := exchange(t, srv, q.Get("code"), verifier)
	if tok.Expire < 0.29 || tok.Expire > 0.31 {
		t.Errorf("expires_in = %v, want 0.3", tok.Expire)
	}
	get := func() int {
		req, _ := http.NewRequest("GET", srv.ResourceURL(), nil)
		req.Header.Set("Authorization", "Bearer "+tok.Access)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if get() != 200 {
		t.Error("a fresh token was refused")
	}
	srv.ExpireAccessTokens()
	if get() != 401 {
		t.Error("an expired token was accepted")
	}
}

func TestBrowserRefusesForeignURLs(t *testing.T) {
	srv := oauthtest.NewServer()
	defer srv.Close()
	err := srv.Browser()("https://phishing.example/authorize?x=1")
	if err == nil || !strings.Contains(err.Error(), "not an URL of the server") {
		t.Fatalf("Browser = %v", err)
	}
}
