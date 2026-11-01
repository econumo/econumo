//go:build upgrade

package upgradetest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type client struct {
	t     *testing.T
	base  string
	token string
}

func (in *instance) client(t *testing.T, token string) *client {
	return &client{t: t, base: in.base, token: token}
}

type response struct {
	status int
	raw    []byte
	data   json.RawMessage
}

func (c *client) do(method, path string, body any) response {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timezone", "UTC")
	req.Header.Set("Accept-Language", "en")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	out := response{status: resp.StatusCode, raw: raw}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &env) == nil {
		out.data = env.Data
	}
	return out
}

// post and get fail the test unless the call succeeds; out receives the
// envelope's data.
func (c *client) post(path string, body, out any) {
	c.t.Helper()
	c.decode(c.must("POST", path, body), out)
}

func (c *client) get(path string, out any) {
	c.t.Helper()
	c.decode(c.must("GET", path, nil), out)
}

// tryPost is post for a route an older release may not have yet: it returns
// false when the route is unknown and fails the test on any other error.
func (c *client) tryPost(path string, body, out any) bool {
	c.t.Helper()
	r := c.do("POST", path, body)
	if r.status == http.StatusNotFound || r.status == http.StatusMethodNotAllowed {
		return false
	}
	if r.status != http.StatusOK {
		c.t.Fatalf("POST %s: HTTP %d: %s", path, r.status, r.raw)
	}
	c.decode(r, out)
	return true
}

func (c *client) must(method, path string, body any) response {
	c.t.Helper()
	r := c.do(method, path, body)
	if r.status != http.StatusOK {
		c.t.Fatalf("%s %s: HTTP %d: %s", method, path, r.status, r.raw)
	}
	return r
}

func (c *client) decode(r response, out any) {
	c.t.Helper()
	if out == nil {
		return
	}
	if err := json.Unmarshal(r.data, out); err != nil {
		c.t.Fatalf("decode %s: %v", r.raw, err)
	}
}

type loginResult struct {
	Token string `json:"token"`
	User  struct {
		Id string `json:"id"`
	} `json:"user"`
}

// login answers with a bare {token, user}, not the envelope.
func (c *client) login(email, password string) loginResult {
	c.t.Helper()
	r := c.do("POST", "/api/v1/user/login-user", map[string]any{"username": email, "password": password})
	if r.status != http.StatusOK {
		c.t.Fatalf("login %s: HTTP %d: %s", email, r.status, r.raw)
	}
	var out loginResult
	if err := json.Unmarshal(r.raw, &out); err != nil || out.Token == "" {
		c.t.Fatalf("login %s: no token in %s (%v)", email, r.raw, err)
	}
	return out
}
