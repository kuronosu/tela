package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// now is the clock of the expiry arithmetic; tests replace it.
var now = time.Now

// Token is what the server issued: an access token, usually a refresh
// token, and when the access token stops working. It prints as
// "[redacted]" with every fmt verb and in log/slog, so a token that ends
// up in a log or an error is not readable.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// String returns "[redacted]".
func (Token) String() string { return "[redacted]" }

// GoString returns "oauth.Token{[redacted]}".
func (Token) GoString() string { return "oauth.Token{[redacted]}" }

// LogValue makes log/slog print "[redacted]".
func (Token) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// Valid reports whether the access token is there and does not expire
// within margin. A token without an expiry does not expire.
func (t Token) Valid(margin time.Duration) bool {
	return t.AccessToken != "" && (t.Expiry.IsZero() || now().Add(margin).Before(t.Expiry))
}

// expired reports whether the access token has stopped working.
func (t Token) expired() bool { return !t.Valid(0) }

// flexSeconds reads a JSON number, or a string holding one, as seconds
// (some servers send expires_in as a string; fractions are accepted).
type flexSeconds float64

func (f *flexSeconds) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("oauth: expires_in %q is not a number", s)
	}
	*f = flexSeconds(v)
	return nil
}

// tokenResponse is the JSON of a token endpoint's answer (RFC 6749 §5.1,
// §5.2).
type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	TokenType    string      `json:"token_type"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    flexSeconds `json:"expires_in"`
	Scope        string      `json:"scope"`

	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorURI         string `json:"error_uri"`
}

// maxResponse caps what is read from a server's answer.
const maxResponse = 1 << 20

// post sends form to endpoint and returns the body of a 2xx answer. An
// error response becomes an *Error.
func (s *Session) post(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	cfg := &s.cfg
	if cfg.ClientSecret != "" && !cfg.SecretInBody {
		req.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape(cfg.ClientSecret))
	}
	resp, err := cfg.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: request to %s: %w", req.URL.Redacted(), unwrapURL(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("oauth: reading the answer of %s: %w", req.URL.Redacted(), err)
	}
	var er tokenResponse
	jsonErr := json.Unmarshal(body, &er)
	if jsonErr == nil && er.Error != "" { // GitHub answers 200 with an error
		return nil, &Error{Code: er.Error, Description: er.ErrorDescription, URI: er.ErrorURI, Status: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code := "server_error"
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			code = "invalid_request"
		}
		return nil, &Error{Code: code, Description: "the answer has no error code", Status: resp.StatusCode}
	}
	return body, nil
}

// unwrapURL drops the *url.Error wrapper, whose message repeats the URL
// (which can carry nothing secret here, but is noise).
func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// tokenRequest sends a token endpoint request and parses the Token.
func (s *Session) tokenRequest(ctx context.Context, form url.Values) (Token, error) {
	form.Set("client_id", s.cfg.ClientID)
	if s.cfg.ClientSecret != "" && s.cfg.SecretInBody {
		form.Set("client_secret", s.cfg.ClientSecret)
	}
	body, err := s.post(ctx, s.cfg.TokenURL, form)
	if err != nil {
		return Token{}, err
	}
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Token{}, fmt.Errorf("oauth: the token endpoint did not answer with JSON: %w", err)
	}
	if r.AccessToken == "" {
		return Token{}, errors.New("oauth: the token endpoint's answer has no access_token")
	}
	if r.TokenType != "" && !strings.EqualFold(r.TokenType, "bearer") {
		return Token{}, fmt.Errorf("oauth: the token type %q is not supported; only Bearer tokens are", r.TokenType)
	}
	t := Token{AccessToken: r.AccessToken, TokenType: "Bearer", RefreshToken: r.RefreshToken, Scope: r.Scope}
	if r.ExpiresIn > 0 {
		t.Expiry = now().Add(time.Duration(float64(r.ExpiresIn) * float64(time.Second)))
	}
	return t, nil
}
