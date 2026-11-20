package oauth_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/oauth"
	"github.com/kuronosu/tela/oauth/oauthtest"
	"github.com/kuronosu/tela/telatest"
)

// State is comparable, so a component can take it as a prop.
var _ = oauth.State{} == oauth.State{}

var statusNames = map[oauth.Status]string{
	oauth.Loading: "loading", oauth.SignedOut: "signed out", oauth.SigningIn: "signing in",
	oauth.SignedIn: "signed in", oauth.Failed: "failed",
}

// sessionView is a component that shows the session's state and has a
// button for each command; seen gets every state it builds with.
func sessionView(cfg oauth.Config, seen func(oauth.State)) func() tela.Node {
	return func() tela.Node {
		st := oauth.UseSession(cfg)
		if seen != nil {
			seen(st)
		}
		label := "status: " + statusNames[st.Status]
		if st.Err != nil {
			label += " (" + st.Err.Error() + ")"
		}
		code := h.Fragment()
		if st.Device != nil {
			code = h.Text("code " + st.Device.UserCode)
		}
		return h.Div(
			h.Text(label), code,
			h.Button(h.OnClick(st.SignIn), h.Text("Sign in")),
			h.Button(h.OnClick(st.SignInWithDevice), h.Text("Sign in with a code")),
			h.Button(h.OnClick(st.Cancel), h.Text("Cancel")),
			h.Button(h.OnClick(st.SignOut), h.Text("Sign out")),
		)
	}
}

func mountSession(t *testing.T, mutate func(*oauth.Config), opts ...oauthtest.Option) (*oauthtest.Server, oauth.Config, *telatest.App) {
	t.Helper()
	srv := oauthtest.NewServer(opts...)
	t.Cleanup(srv.Close)
	cfg := srv.Config()
	if mutate != nil {
		mutate(&cfg)
	}
	app := telatest.MountFunc(t, sessionView(cfg, nil), telatest.Timeout(10*time.Second))
	return srv, cfg, app
}

func TestUseSessionSignInAndOut(t *testing.T) {
	srv, cfg, app := mountSession(t, nil)
	app.ExpectText("status: signed out")

	app.Click(telatest.ByRole("button", "Sign in"))
	app.ExpectText("status: signed in")
	if _, ok := app.StoredSecret(cfg.SecretName); !ok {
		t.Error("the tokens were not stored")
	}

	app.Click(telatest.ByRole("button", "Sign out"))
	app.ExpectText("status: signed out")
	app.WaitFor("the stored tokens deleted", func() bool { _, ok := app.StoredSecret(cfg.SecretName); return !ok })
	if srv.RevokeRequests() == 0 {
		t.Error("the tokens were not revoked")
	}
}

// The session reads the tokens an earlier run stored.
func TestUseSessionLoadsStoredTokens(t *testing.T) {
	srv := oauthtest.NewServer()
	t.Cleanup(srv.Close)
	cfg := srv.Config()
	view := sessionView(cfg, nil)
	app := telatest.MountFunc(t, func() tela.Node {
		show, setShow := tela.UseState(false)
		return h.Div(
			h.Button(h.OnClick(func() { setShow.Set(true) }), h.Text("Show")),
			h.If(show, tela.C(view)),
		)
	}, telatest.Timeout(10*time.Second))
	app.SetStoredSecret(cfg.SecretName, mustJSON(oauth.Token{AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour)}))
	app.Click(telatest.ByRole("button", "Show"))
	app.ExpectText("status: signed in")
	if n := srv.Calls("refresh_token"); n != 0 {
		t.Errorf("%d refreshes at start", n)
	}
}

func TestUseSessionDenied(t *testing.T) {
	srv, _, app := mountSession(t, nil)
	srv.SetMode(oauthtest.Deny)
	app.ExpectText("status: signed out")
	app.Click(telatest.ByRole("button", "Sign in"))
	app.ExpectText("status: failed")
	app.ExpectText("access_denied")
}

func TestUseSessionCancel(t *testing.T) {
	cap := newCapture()
	_, _, app := mountSession(t, func(c *oauth.Config) { c.Open = cap.open })
	app.ExpectText("status: signed out")
	app.Click(telatest.ByRole("button", "Sign in"))
	app.ExpectText("status: signing in")
	addr := listenerOf(t, cap.next(t))

	app.Click(telatest.ByRole("button", "Cancel"))
	app.ExpectText("status: signed out")
	if !closed(addr) {
		t.Error("the listener stayed open after Cancel")
	}
}

func TestUseSessionUnmountCancels(t *testing.T) {
	cap := newCapture()
	_, _, app := mountSession(t, func(c *oauth.Config) { c.Open = cap.open })
	app.ExpectText("status: signed out")
	app.Click(telatest.ByRole("button", "Sign in"))
	app.ExpectText("status: signing in")
	addr := listenerOf(t, cap.next(t))

	app.Close()
	if !closed(addr) {
		t.Error("the listener stayed open after the component unmounted")
	}
}

func TestUseSessionDevice(t *testing.T) {
	srv, _, app := mountSession(t, nil, oauthtest.DeviceInterval(30*time.Millisecond))
	srv.PendingPolls(5)
	app.ExpectText("status: signed out")
	app.Click(telatest.ByRole("button", "Sign in with a code"))
	app.ExpectText("code WDJB-MJHT")
	app.ExpectText("status: signed in")
}

// A refresh the server refuses signs the component out.
func TestUseSessionFollowsRefreshFailure(t *testing.T) {
	var clk clock
	oauth.SetNow(t, clk.now)
	srv := oauthtest.NewServer()
	t.Cleanup(srv.Close)
	cfg := srv.Config()
	var last atomic.Pointer[oauth.State]
	app := telatest.MountFunc(t, sessionView(cfg, func(s oauth.State) { last.Store(&s) }), telatest.Timeout(10*time.Second))
	app.Click(telatest.ByRole("button", "Sign in"))
	app.ExpectText("status: signed in")

	// Another session of the same account rotates the refresh token...
	clk.advance(2 * time.Hour)
	other := oauth.New(cfg)
	if _, err := tokenOf(t, other); err != nil {
		t.Fatal(err)
	}
	// ...so the component's own, holding the used one, is a reuse.
	s := last.Load().Session()
	if _, err := call(func() (oauth.Token, error) { return s.Token(context.Background()) }); err == nil {
		t.Fatal("the stale session refreshed")
	}
	app.ExpectText("status: signed out")
}
