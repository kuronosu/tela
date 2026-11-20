package oauthtest_test

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela/oauth/oauthtest"
)

// A test of an app that signs in: start the server, give the app its
// Config, and choose how the user answers. The app must be running
// (telatest.Mount) for the tokens' secure storage.
//
//	func TestSignIn(t *testing.T) {
//	    srv := oauthtest.NewServer()
//	    defer srv.Close()
//	    app := telatest.MountFunc(t, root(srv.Config()))
//	    app.Click(telatest.ByRole("button", "Sign in"))
//	    app.ExpectText("Welcome back")
//
//	    srv.SetMode(oauthtest.Deny) // the user says no next time
//	}
func ExampleNewServer() {
	srv := oauthtest.NewServer(oauthtest.TokenLifetime(time.Minute))
	defer srv.Close()
	cfg := srv.Config()
	fmt.Println(cfg.ClientID, cfg.Open != nil, cfg.AuthURL == srv.URL+"/authorize")
	// Output: oauthtest-client true true
}
