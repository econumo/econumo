package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	case "EC":
		if k.Crv != "P-256" {
			return nil, fmt.Errorf("oidc: unsupported curve %q", k.Crv)
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		y, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}, nil
	}
	return nil, fmt.Errorf("oidc: unsupported kty %q", k.Kty)
}

// minJWKSRefresh bounds how often the SAME unknown kid may trigger a
// re-fetch, so an attacker replaying one forged kid cannot turn the verifier
// into a JWKS hammer; a genuinely new kid (a real rotation) always gets one
// immediate refetch attempt.
const minJWKSRefresh = time.Minute

// keyFor resolves the signing key for kid, re-fetching the JWKS once on a miss.
func (c *Client) keyFor(ctx context.Context, kid string) (crypto.PublicKey, error) {
	c.mu.Lock()
	if k, ok := c.keys[kid]; ok {
		c.mu.Unlock()
		return k, nil
	}
	throttled := c.lastMissKid == kid && time.Since(c.lastMissAt) < minJWKSRefresh
	c.mu.Unlock()
	if throttled {
		return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
	}
	if err := c.fetchJWKS(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	c.lastMissKid = kid
	c.lastMissAt = time.Now()
	return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
}

func (c *Client) fetchJWKS(ctx context.Context) error {
	disc, err := c.Discover(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, disc.JWKSURI, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: jwks returned status %d", resp.StatusCode)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return err
	}
	keys := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, perr := k.publicKey()
		if perr != nil {
			continue // one exotic key must not poison the set
		}
		keys[k.Kid] = pub
	}
	c.mu.Lock()
	c.keys = keys
	c.keysFetched = time.Now()
	c.mu.Unlock()
	return nil
}
