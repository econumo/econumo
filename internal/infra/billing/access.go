// Package billing is Econumo's client of the billing portal's server API. It
// lives in infra so the user feature reaches billing through a port.
package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

// Timeout bounds one get-access call: the SPA waits on it for a settings
// label, so a slow billing must not hold the request.
const Timeout = 3 * time.Second

const maxResponseBytes = 1 << 20

const accessPath = "/api/v1/econumo/get-access"

// AccessSigner mints the per-user bearer billing verifies
// (handoff.Signer.SignAccess).
type AccessSigner func(uid string) (string, error)

type AccessClient struct {
	endpoint string
	sign     AccessSigner
	http     *http.Client
}

// NewAccessClient targets billingURL's get-access, keeping any path prefix
// and query of ECONUMO_BILLING_URL. hc nil uses a client with Timeout.
func NewAccessClient(billingURL string, sign AccessSigner, hc *http.Client) (*AccessClient, error) {
	u, err := url.Parse(billingURL)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + accessPath
	u.RawPath = ""
	if hc == nil {
		hc = &http.Client{Timeout: Timeout}
	}
	return &AccessClient{endpoint: u.String(), sign: sign, http: hc}, nil
}

type accessRequest struct {
	Connections []string `json:"connections"`
}

type accessEnvelope struct {
	Success bool `json:"success"`
	Data    *struct {
		User        *model.BillingAccessUser        `json:"user"`
		Connections []model.BillingAccessConnection `json:"connections"`
	} `json:"data"`
}

// ErrBadResponse is any answer that is not a successful envelope carrying a
// well-formed view: a missing route on an older billing, an error envelope,
// malformed JSON or dates.
var ErrBadResponse = errors.New("billing: bad get-access response")

// GetAccess asks billing for uid's subscription view and the given
// connections'. The caller decides who the connections are.
func (c *AccessClient) GetAccess(ctx context.Context, uid string, connectionIDs []string) (model.BillingAccessUser, []model.BillingAccessConnection, error) {
	token, err := c.sign(uid)
	if err != nil {
		return model.BillingAccessUser{}, nil, err
	}
	if connectionIDs == nil {
		connectionIDs = []string{}
	}
	body, err := json.Marshal(accessRequest{Connections: connectionIDs})
	if err != nil {
		return model.BillingAccessUser{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return model.BillingAccessUser{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return model.BillingAccessUser{}, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return model.BillingAccessUser{}, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return model.BillingAccessUser{}, nil, fmt.Errorf("%w: HTTP %d", ErrBadResponse, resp.StatusCode)
	}
	var env accessEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return model.BillingAccessUser{}, nil, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	if !env.Success || env.Data == nil || env.Data.User == nil {
		return model.BillingAccessUser{}, nil, fmt.Errorf("%w: not a successful envelope", ErrBadResponse)
	}
	u := *env.Data.User
	for _, d := range []string{u.RenewsAt, u.EndsAt, u.DiscountUntil} {
		if !validDatetime(d) {
			return model.BillingAccessUser{}, nil, fmt.Errorf("%w: malformed date", ErrBadResponse)
		}
	}
	return u, env.Data.Connections, nil
}

func validDatetime(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.ParseInLocation(datetime.Layout, s, time.UTC)
	return err == nil
}
