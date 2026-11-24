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

// Every field is a pointer so a missing one is told from a zero value: a
// partial answer must not read as "unpaid, not renewing".
type accessEnvelope struct {
	Success bool `json:"success"`
	Data    *struct {
		User *struct {
			Paid          *bool   `json:"paid"`
			Renewing      *bool   `json:"renewing"`
			RenewsAt      *string `json:"renewsAt"`
			EndsAt        *string `json:"endsAt"`
			PaymentFailed *bool   `json:"paymentFailed"`
			DiscountUntil *string `json:"discountUntil"`
		} `json:"user"`
		Connections *[]struct {
			Id       *string `json:"id"`
			Paid     *bool   `json:"paid"`
			Renewing *bool   `json:"renewing"`
		} `json:"connections"`
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
	if !env.Success || env.Data == nil || env.Data.User == nil || env.Data.Connections == nil {
		return model.BillingAccessUser{}, nil, fmt.Errorf("%w: not a successful envelope", ErrBadResponse)
	}
	ru := env.Data.User
	if ru.Paid == nil || ru.Renewing == nil || ru.RenewsAt == nil || ru.EndsAt == nil || ru.PaymentFailed == nil || ru.DiscountUntil == nil {
		return model.BillingAccessUser{}, nil, fmt.Errorf("%w: incomplete user", ErrBadResponse)
	}
	u := model.BillingAccessUser{Paid: *ru.Paid, Renewing: *ru.Renewing, RenewsAt: *ru.RenewsAt, EndsAt: *ru.EndsAt,
		PaymentFailed: *ru.PaymentFailed, DiscountUntil: *ru.DiscountUntil}
	for _, d := range []string{u.RenewsAt, u.EndsAt, u.DiscountUntil} {
		if !validDatetime(d) {
			return model.BillingAccessUser{}, nil, fmt.Errorf("%w: malformed date", ErrBadResponse)
		}
	}
	conns := make([]model.BillingAccessConnection, 0, len(*env.Data.Connections))
	for _, c := range *env.Data.Connections {
		if c.Id == nil || c.Paid == nil || c.Renewing == nil {
			return model.BillingAccessUser{}, nil, fmt.Errorf("%w: incomplete connection", ErrBadResponse)
		}
		conns = append(conns, model.BillingAccessConnection{Id: *c.Id, Paid: *c.Paid, Renewing: *c.Renewing})
	}
	return u, conns, nil
}

func validDatetime(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.ParseInLocation(datetime.Layout, s, time.UTC)
	return err == nil
}
