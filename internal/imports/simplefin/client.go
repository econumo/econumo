// Package simplefin is the SimpleFIN bridge client (protocol v1): a setup
// token is exchanged once for an access URL, and every later call is a GET
// against that URL with its embedded HTTP Basic credentials.
package simplefin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
)

const maxBody = 16 << 20

type Options struct {
	AllowHTTP bool
	// AllowPrivateHosts lifts the SSRF guard below. Off in production: a
	// self-hosted bridge on a LAN needs it on.
	AllowPrivateHosts bool
	Client            *http.Client
}

type Client struct {
	http      *http.Client
	allowHTTP bool
}

var (
	_ imports.Provider          = (*Client)(nil)
	_ imports.SetupTokenClaimer = (*Client)(nil)
)

func New(opts Options) *Client {
	c := opts.Client
	if c == nil {
		c = &http.Client{
			Timeout: 60 * time.Second,
			// A redirect could send the Basic credentials to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if !opts.AllowPrivateHosts {
		guarded := *c
		guarded.Transport = guardTransport(c.Transport)
		c = &guarded
	}
	return &Client{http: c, allowHTTP: opts.AllowHTTP}
}

// errBlockedAddress is deliberately static: it travels back out of
// http.Client.Do, and anything derived from the request would carry the
// access URL and its embedded credentials.
var errBlockedAddress = errors.New("bridge address is not publicly routable")

// guardTransport refuses every address that is not globally routable. Both
// the setup token and the access URL are supplied by the user, so without it
// the server fetches whatever its own network position can reach on request:
// loopback admin ports, LAN services, cloud metadata endpoints.
func guardTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	t, ok := base.(*http.Transport)
	if !ok {
		// No dialer to install on a caller's own round tripper, so the host
		// is resolved and checked before the request is handed over.
		return checkedRoundTripper{next: base}
	}
	clone := t.Clone()
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errBlockedAddress
		}
		ips, err := resolveAllowed(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error = errBlockedAddress
		for _, a := range ips {
			// Dial the address that was just checked rather than the name:
			// a second resolution reopens the window a rebinding answer needs.
			conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
			if derr == nil {
				return conn, nil
			}
			lastErr = derr
		}
		return nil, lastErr
	}
	return clone
}

type checkedRoundTripper struct{ next http.RoundTripper }

func (c checkedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := resolveAllowed(req.Context(), req.URL.Hostname()); err != nil {
		return nil, err
	}
	return c.next.RoundTrip(req)
}

func resolveAllowed(ctx context.Context, host string) ([]net.IPAddr, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errBlockedAddress
	}
	for _, a := range ips {
		// One blocked answer condemns the whole host: otherwise a rebinding
		// record only has to win a retry.
		if !a.IP.IsGlobalUnicast() || a.IP.IsPrivate() {
			return nil, errBlockedAddress
		}
	}
	return ips, nil
}

func (c *Client) ClaimSetupToken(ctx context.Context, setupToken string) (string, error) {
	claimURL, err := c.decodeSetupToken(setupToken)
	if err != nil {
		return "", imports.ErrSetupTokenRejected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, http.NoBody)
	if err != nil {
		return "", imports.ErrSetupTokenRejected
	}
	req.Header.Set("Content-Length", "0")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", imports.ErrProviderUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", imports.ErrProviderUnavailable
	}
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return "", imports.ErrSetupTokenRejected
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", imports.ErrProviderUnavailable
	}
	accessURL := strings.TrimSpace(string(body))
	if _, _, err := c.splitAccessURL(accessURL); err != nil {
		return "", imports.ErrProviderUnavailable
	}
	return accessURL, nil
}

func (c *Client) ListAccounts(ctx context.Context, cred imports.Credential) ([]model.ExternalAccount, error) {
	res, err := c.fetch(ctx, cred, url.Values{"balances-only": {"1"}})
	if err != nil {
		return nil, err
	}
	return res.Accounts, nil
}

func (c *Client) FetchTransactions(ctx context.Context, cred imports.Credential, req imports.FetchRequest) (*imports.FetchResult, error) {
	q := url.Values{}
	if !req.StartDate.IsZero() {
		q.Set("start-date", strconv.FormatInt(req.StartDate.Unix(), 10))
	}
	if !req.EndDate.IsZero() {
		q.Set("end-date", strconv.FormatInt(req.EndDate.Unix(), 10))
	}
	return c.fetch(ctx, cred, q)
}

func (c *Client) decodeSetupToken(tok string) (string, error) {
	tok = strings.TrimSpace(tok)
	raw, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		// tokens are often pasted without padding
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(tok, "="))
		if err != nil {
			return "", err
		}
	}
	u, err := url.Parse(strings.TrimSpace(string(raw)))
	if err != nil || u.Host == "" || !c.schemeOK(u.Scheme) {
		return "", errors.New("claim url invalid")
	}
	return u.String(), nil
}

func (c *Client) schemeOK(scheme string) bool {
	return scheme == "https" || (c.allowHTTP && scheme == "http")
}

// splitAccessURL separates the Basic credentials from the base URL so they
// travel in the Authorization header rather than the request line.
func (c *Client) splitAccessURL(raw string) (base *url.URL, user *url.Userinfo, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !c.schemeOK(u.Scheme) || u.User == nil {
		return nil, nil, imports.ErrCredentialInvalid
	}
	if _, ok := u.User.Password(); !ok {
		return nil, nil, imports.ErrCredentialInvalid
	}
	user = u.User
	u.User = nil
	return u, user, nil
}

type document struct {
	Errors   []json.RawMessage `json:"errors"`
	Accounts []struct {
		Org struct {
			Name string `json:"name"`
		} `json:"org"`
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		Currency     string            `json:"currency"`
		Balance      string            `json:"balance"`
		Transactions []json.RawMessage `json:"transactions"`
	} `json:"accounts"`
}

type transaction struct {
	ID          string `json:"id"`
	Posted      int64  `json:"posted"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
	Payee       string `json:"payee"`
	Pending     bool   `json:"pending"`
}

func (c *Client) fetch(ctx context.Context, cred imports.Credential, q url.Values) (*imports.FetchResult, error) {
	base, user, err := c.splitAccessURL(cred.AccessURL)
	if err != nil {
		return nil, err
	}
	endpoint := *base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/accounts"
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, imports.ErrCredentialInvalid
	}
	pass, _ := user.Password()
	req.SetBasicAuth(user.Username(), pass)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// never wrap err: url.Error carries the full URL, credentials included
		return nil, imports.ErrProviderUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		// The bridge answers these once the access URL is revoked or its
		// credentials stop matching: retrying never recovers, the user has to
		// reconnect, so this must not read as a transient outage.
		drain(resp.Body)
		return nil, imports.ErrCredentialInvalid
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		drain(resp.Body)
		return nil, fmt.Errorf("%w: status %d", imports.ErrProviderUnavailable, resp.StatusCode)
	}
	var doc document
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&doc); err != nil {
		return nil, imports.ErrProviderUnavailable
	}
	out := &imports.FetchResult{Accounts: []model.ExternalAccount{}, Transactions: []model.ExternalTransaction{}, Warnings: warnings(doc.Errors)}
	for _, a := range doc.Accounts {
		out.Accounts = append(out.Accounts, model.ExternalAccount{ID: a.ID, Name: a.Name, Currency: a.Currency, Balance: a.Balance, OrgName: a.Org.Name})
		for _, raw := range a.Transactions {
			var t transaction
			if err := json.Unmarshal(raw, &t); err != nil {
				// A row the client cannot read is still forwarded so the
				// parser records it as a failed event: dropping it here would
				// lose it from the run with nothing to show the user.
				out.Transactions = append(out.Transactions, model.ExternalTransaction{ExternalAccountID: a.ID, Raw: raw})
				continue
			}
			if t.Pending {
				continue // pending rows change id and amount when they settle
			}
			out.Transactions = append(out.Transactions, model.ExternalTransaction{
				ExternalAccountID: a.ID, ID: t.ID, Amount: t.Amount, Posted: t.Posted, Payee: t.Payee, Description: t.Description, Raw: raw,
			})
		}
	}
	return out, nil
}

// drain keeps the connection reusable: an unread body forces a new one.
func drain(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4<<10))
}

// warnings accepts the v1 list of strings and, defensively, objects with a
// "message" key.
func warnings(raw []json.RawMessage) []string {
	out := []string{}
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, s)
			continue
		}
		var o struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(r, &o) == nil && o.Message != "" {
			out = append(out, o.Message)
		}
	}
	return out
}
