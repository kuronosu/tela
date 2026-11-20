package oauth

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config describes how an app signs in to one OAuth 2.0 server. The zero
// values of the optional fields are the right ones for a desktop app.
type Config struct {
	// ClientID identifies the app to the server. A desktop app is a
	// public client (RFC 8252 §8.5): it has no secret, and PKCE protects
	// the code instead.
	ClientID string
	// ClientSecret is optional, for servers that insist on one (RFC 8252
	// §8.5 explains why it cannot stay secret in an app that is
	// distributed). It is sent with HTTP Basic, or in the body with
	// SecretInBody.
	ClientSecret string
	SecretInBody bool

	// AuthURL is the authorization endpoint, TokenURL the token endpoint.
	// DeviceURL (the device authorization endpoint, RFC 8628) is needed
	// for DeviceSignIn only, and RevokeURL (RFC 7009) is optional: with
	// it SignOut also revokes the tokens at the server. They must be
	// https URLs; http is accepted for a loopback host only (tests and
	// local servers).
	AuthURL   string
	TokenURL  string
	DeviceURL string
	RevokeURL string

	// Scopes are the requested scopes, joined with spaces.
	Scopes []string
	// Params are extra parameters of the authorization request, such as
	// "prompt" or "login_hint". The ones the flow owns (client_id,
	// redirect_uri, response_type, scope, state, code_challenge,
	// code_challenge_method) cannot be set here.
	Params map[string]string

	// SecretName is the name of the secret the tokens are kept under
	// (desktop.SetSecret); one per account the app signs in to. Default:
	// "oauth/" and the client ID.
	SecretName string

	// RedirectPath is the path of the redirect URI on the loopback
	// listener: "http://127.0.0.1:<port>" and this path. Default
	// "/callback". Some servers register the exact path.
	RedirectPath string

	// Open opens the authorization URL in the user's browser. Default: the
	// system browser (tela.OpenURL, on the UI thread). Tests use
	// oauthtest.Server.Browser.
	Open func(url string) error

	// Page writes the page the browser shows when the sign-in ends: err is
	// nil when it worked. Default: a small page that says "You can close
	// this window." Page runs once, in a handler of the loopback listener;
	// it writes the response like an http.Handler (the default sets a
	// Content-Security-Policy; set one too).
	Page func(w http.ResponseWriter, err error)

	// HTTPClient makes the requests to the server (token, device,
	// revocation). Default: a client with a 30 s timeout that does not
	// follow redirects.
	HTTPClient *http.Client

	// RefreshMargin is how long before it expires an access token is
	// refreshed. Default one minute.
	RefreshMargin time.Duration
}

func (c *Config) secretName() string {
	if c.SecretName != "" {
		return c.SecretName
	}
	return "oauth/" + c.ClientID
}

func (c *Config) redirectPath() string {
	if c.RedirectPath != "" {
		return c.RedirectPath
	}
	return "/callback"
}

func (c *Config) margin() time.Duration {
	if c.RefreshMargin > 0 {
		return c.RefreshMargin
	}
	return time.Minute
}

// reserved are the authorization parameters the flow sets itself.
var reserved = []string{"client_id", "redirect_uri", "response_type", "scope", "state", "code_challenge", "code_challenge_method"}

// validate returns the first problem of the Config that every flow needs.
func (c *Config) validate() error {
	if c.ClientID == "" {
		return fmt.Errorf("oauth: Config.ClientID is empty; use the client ID the server gave the app")
	}
	if len(c.secretName()) > 255 {
		return fmt.Errorf("oauth: Config.SecretName is longer than 255 bytes; use a short name such as %q", "github-token")
	}
	if c.RedirectPath != "" && !strings.HasPrefix(c.RedirectPath, "/") {
		return fmt.Errorf("oauth: Config.RedirectPath %q must start with a slash", c.RedirectPath)
	}
	for _, k := range reserved {
		if _, ok := c.Params[k]; ok {
			return fmt.Errorf("oauth: Config.Params[%q] is set by the sign-in itself; remove it (use Scopes for the scope)", k)
		}
	}
	for _, e := range []struct{ name, u string }{{"AuthURL", c.AuthURL}, {"TokenURL", c.TokenURL}, {"DeviceURL", c.DeviceURL}, {"RevokeURL", c.RevokeURL}} {
		if e.u == "" {
			continue
		}
		if err := checkEndpoint(e.u); err != nil {
			return fmt.Errorf("oauth: Config.%s: %w", e.name, err)
		}
	}
	return nil
}

// need returns an error naming the endpoint a flow needs and the Config
// does not have.
func (c *Config) need(flow, field, value string) error {
	if value == "" {
		return fmt.Errorf("oauth: %s needs Config.%s; set it to the server's endpoint (https://…)", flow, field)
	}
	return nil
}

// checkEndpoint accepts an absolute https URL, or http for a loopback
// host.
func checkEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not an absolute URL; write it whole, such as \"https://example.com/oauth/token\"", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopback(u.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("%q must be an https URL (tokens and codes must not cross the network in the clear)", raw)
}

// isLoopback reports whether host is localhost or a loopback address.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// client returns the client for the requests to the server.
func (c *Config) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

var defaultHTTPClient = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
