// Package oauth signs a desktop app's user in to an OAuth 2.0 server and
// keeps the tokens (ADR-298).
//
// It follows RFC 8252 (OAuth for native apps) and the security practice of
// RFC 9700: the authorization code flow with PKCE (RFC 7636, S256) and a
// random state, through the system browser and a redirect to a loopback
// listener on 127.0.0.1 at a free port, so the app needs no secret of its
// own; and the device flow (RFC 8628) for a sign-in that happens on another
// device. Only the standard library is used.
//
//	var github = oauth.Config{
//	    ClientID:   "Iv1.0123456789abcdef",
//	    AuthURL:    "https://github.com/login/oauth/authorize",
//	    TokenURL:   "https://github.com/login/oauth/access_token",
//	    Scopes:     []string{"read:user"},
//	    SecretName: "github-token",
//	}
//
// A [Session] is one signed-in account of a [Config]. [New] makes it;
// [Session.SignIn] runs the flow, [Session.Client] gives an HTTP client
// that sends the access token and refreshes it, and [Session.SignOut]
// forgets it. Components use [UseSession], which does all of it off the UI
// thread and returns a comparable [State].
//
// # Tokens
//
// The tokens are kept as one JSON secret in the OS's secure storage
// ([desktop.SetSecret]), under Config.SecretName, and loaded when the
// session is first used; they need a running app (tela.Run or telatest).
// A token is refreshed shortly before it expires; a rotated refresh token
// is stored; simultaneous callers share one refresh; a refresh the server
// rejects with invalid_grant means the grant is gone (revoked, or a used
// refresh token came back: RFC 9700 §4.14), and the session is signed out
// ([ErrSignedOut]). A token never appears in an error message or a log:
// [Token] prints as "[redacted]".
//
// # Testing
//
// Package oauthtest is a fake server that approves, denies or expires,
// with a fake browser, for the tests of the app and of telatest.
package oauth
