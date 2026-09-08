package oidc

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Issuer struct {
	ID              string
	IssuerURL       string
	ClientID        string
	ClientSecret    func(now time.Time) (string, error)
	Scopes          []string
	UsePKCE         bool
	ResponseMode    string
	TrustEmail      bool
	ExtraAuthParams map[string]string
}

// StaticSecret adapts a plain client secret to the ClientSecret seam.
func StaticSecret(secret string) func(time.Time) (string, error) {
	return func(time.Time) (string, error) { return secret, nil }
}

type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

type Tokens struct {
	IDToken     string
	AccessToken string
}

type Client struct {
	issuer Issuer
	http   *http.Client

	mu         sync.Mutex
	disc       *Discovery
	keys       map[string]crypto.PublicKey
	lastMissAt time.Time
}

func NewClient(issuer Issuer, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	issuer.IssuerURL = strings.TrimSuffix(issuer.IssuerURL, "/")
	return &Client{issuer: issuer, http: hc}
}

func (c *Client) Issuer() Issuer { return c.issuer }

// Discover loads the OpenID configuration once per process (lazily), so an
// issuer that is down at boot only fails the requests that need it.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	c.mu.Lock()
	if c.disc != nil {
		d := *c.disc
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.issuer.IssuerURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return Discovery{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Discovery{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Discovery{}, fmt.Errorf("oidc: discovery returned status %d", resp.StatusCode)
	}
	var d Discovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return Discovery{}, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return Discovery{}, errors.New("oidc: discovery document lacks required endpoints")
	}
	// A document served from the configured URL but claiming another issuer is
	// either a misconfiguration or a redirect to a foreign IdP; its endpoints
	// must not be trusted, and its ID tokens would pass the issuer check below.
	if iss := strings.TrimSuffix(d.Issuer, "/"); iss != c.issuer.IssuerURL {
		return Discovery{}, fmt.Errorf("oidc: discovery issuer %q does not match the configured issuer %q", d.Issuer, c.issuer.IssuerURL)
	}
	c.mu.Lock()
	c.disc = &d
	c.mu.Unlock()
	return d, nil
}

func (c *Client) AuthURL(ctx context.Context, state, nonce, codeChallenge, redirectURI string) (string, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.issuer.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", strings.Join(c.issuer.Scopes, " "))
	q.Set("state", state)
	q.Set("nonce", nonce)
	if c.issuer.UsePKCE {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	if c.issuer.ResponseMode != "" {
		q.Set("response_mode", c.issuer.ResponseMode)
	}
	for k, v := range c.issuer.ExtraAuthParams {
		q.Set(k, v)
	}
	sep := "?"
	if strings.Contains(d.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return d.AuthorizationEndpoint + sep + q.Encode(), nil
}

// Exchange redeems the code with client_secret_post authentication.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier, redirectURI string, now time.Time) (Tokens, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return Tokens{}, err
	}
	secret, err := c.issuer.ClientSecret(now)
	if err != nil {
		return Tokens{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", c.issuer.ClientID)
	form.Set("client_secret", secret)
	if c.issuer.UsePKCE {
		form.Set("code_verifier", codeVerifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		IDToken     string `json:"id_token"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Tokens{}, fmt.Errorf("oidc: token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.Error != "" {
		return Tokens{}, fmt.Errorf("oidc: token endpoint status %d error %q", resp.StatusCode, body.Error)
	}
	if body.IDToken == "" {
		return Tokens{}, errors.New("oidc: token response carries no id_token")
	}
	return Tokens{IDToken: body.IDToken, AccessToken: body.AccessToken}, nil
}

func (c *Client) VerifyIDToken(ctx context.Context, raw, nonce string, now time.Time) (Claims, error) {
	h, cl, input, sig, err := parseCompact(raw)
	if err != nil {
		return Claims{}, err
	}
	if h.Alg != "RS256" && h.Alg != "ES256" {
		return Claims{}, fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, h.Alg)
	}
	key, err := c.keyFor(ctx, h.Kid)
	if err != nil {
		return Claims{}, err
	}
	if err := verifySignature(h.Alg, key, input, sig); err != nil {
		return Claims{}, err
	}
	d, err := c.Discover(ctx)
	if err != nil {
		return Claims{}, err
	}
	iss := strings.TrimSuffix(cl.Iss, "/")
	if iss != strings.TrimSuffix(d.Issuer, "/") && iss != c.issuer.IssuerURL {
		return Claims{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, cl.Iss)
	}
	if !audienceContains(cl.Aud, c.issuer.ClientID) {
		return Claims{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	if cl.Exp == 0 || now.After(time.Unix(cl.Exp, 0).Add(clockSkew)) {
		return Claims{}, fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	if cl.Iat != 0 && time.Unix(cl.Iat, 0).After(now.Add(clockSkew)) {
		return Claims{}, fmt.Errorf("%w: issued in the future", ErrInvalidToken)
	}
	if nonce != "" && cl.Nonce != nonce {
		return Claims{}, fmt.Errorf("%w: nonce", ErrInvalidToken)
	}
	if cl.Sub == "" {
		return Claims{}, fmt.Errorf("%w: missing sub", ErrInvalidToken)
	}
	return cl.toClaims(), nil
}

// UserInfo fetches the userinfo document; the caller decides how to merge it.
func (c *Client) UserInfo(ctx context.Context, accessToken string) (Claims, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return Claims{}, err
	}
	if d.UserinfoEndpoint == "" {
		return Claims{}, errors.New("oidc: issuer publishes no userinfo endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.UserinfoEndpoint, nil)
	if err != nil {
		return Claims{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Claims{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Claims{}, fmt.Errorf("oidc: userinfo returned status %d", resp.StatusCode)
	}
	var cl idTokenClaims
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cl); err != nil {
		return Claims{}, err
	}
	return cl.toClaims(), nil
}

// EndSessionURL builds the RP-initiated logout URL, or "" when the issuer
// publishes no end_session_endpoint.
func (c *Client) EndSessionURL(ctx context.Context, idToken, postLogoutRedirectURI string) (string, error) {
	d, err := c.Discover(ctx)
	if err != nil {
		return "", err
	}
	if d.EndSessionEndpoint == "" {
		return "", nil
	}
	q := url.Values{}
	q.Set("id_token_hint", idToken)
	q.Set("client_id", c.issuer.ClientID)
	q.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	sep := "?"
	if strings.Contains(d.EndSessionEndpoint, "?") {
		sep = "&"
	}
	return d.EndSessionEndpoint + sep + q.Encode(), nil
}
