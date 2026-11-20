package oauth_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/oauth"
)

// github is a package-level Config, like an update.Config. A desktop app
// has no client secret: PKCE protects the code.
var github = oauth.Config{
	ClientID:   "Iv1.0123456789abcdef",
	AuthURL:    "https://github.com/login/oauth/authorize",
	TokenURL:   "https://github.com/login/oauth/access_token",
	Scopes:     []string{"read:user"},
	SecretName: "github-token",
}

// A component signs in with UseSession: the browser opens, the user
// approves, and the tokens are kept in the OS's secure storage for the
// next start.
func ExampleUseSession() {
	root := func() tela.Node {
		acct := oauth.UseSession(github)
		switch acct.Status {
		case oauth.SignedIn:
			return Div(Text("Welcome back."), Button(OnClick(acct.SignOut), Text("Sign out")))
		case oauth.SigningIn:
			return Div(Text("Finish in your browser…"), Button(OnClick(acct.Cancel), Text("Cancel")))
		case oauth.Failed:
			return Div(Textf("Could not sign in: %v", acct.Err), Button(OnClick(acct.SignIn), Text("Try again")))
		}
		return Button(OnClick(acct.SignIn), Text("Sign in with GitHub"))
	}
	tela.Main(tela.App{AppID: "com.example.notes", Title: "Notes", Root: tela.C(root)})
}

// Outside a component: sign in, then call the API with a client that sends
// and refreshes the token. Run it on a goroutine (tela.UseGo).
func ExampleSession_SignIn() {
	account := oauth.New(github)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch err := account.SignIn(ctx); {
	case errors.Is(err, oauth.ErrAccessDenied):
		fmt.Println("the user said no")
	case err != nil:
		fmt.Println("sign-in failed:", err)
	default:
		resp, err := account.Client(ctx).Get("https://api.github.com/user")
		if err == nil {
			resp.Body.Close()
		}
	}
}

// The device flow, for an app whose user signs in on a phone.
func ExampleSession_DeviceSignIn() {
	account := oauth.New(oauth.Config{
		ClientID:  "my-client-id",
		DeviceURL: "https://accounts.example.com/device/code",
		TokenURL:  "https://accounts.example.com/oauth/token",
	})
	err := account.DeviceSignIn(context.Background(), func(dc oauth.DeviceCode) {
		fmt.Printf("Go to %s and enter %s\n", dc.VerificationURI, dc.UserCode)
	})
	if errors.Is(err, oauth.ErrExpired) {
		fmt.Println("too late: ask for a new code")
	}
}

// A token prints as "[redacted]", so it cannot end up in a log by mistake.
func ExampleToken() {
	tok := oauth.Token{AccessToken: "ya29.secret", RefreshToken: "1//secret"}
	fmt.Println(tok)
	fmt.Printf("%+v\n", tok)
	// Output:
	// [redacted]
	// [redacted]
}
