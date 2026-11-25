package server

import (
	"net/http"

	"github.com/econumo/econumo/internal/config"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/billing"
	"github.com/econumo/econumo/internal/infra/handoff"
	"github.com/econumo/econumo/internal/shared/port"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

var (
	_ appuser.ConnectedUsers       = (*connectionrepo.Repo)(nil)
	_ appuser.AccessHeaders        = (*userrepo.Repo)(nil)
	_ appuser.BillingAccessFetcher = (*billing.AccessClient)(nil)
)

// newBillingAccessService wires get-billing-access; without ECONUMO_BILLING_URL
// it has no fetcher and never calls out.
func newBillingAccessService(cfg config.Config, users *userrepo.Repo, conns *connectionrepo.Repo, clk port.Clock, hc *http.Client) (*appuser.BillingAccessService, error) {
	if cfg.BillingURL == "" {
		return appuser.NewBillingAccessService(nil, conns, users, clk), nil
	}
	signer := handoff.NewSigner(cfg.AdminToken)
	client, err := billing.NewAccessClient(cfg.BillingURL, func(uid string) (string, error) {
		return signer.SignAccess(uid, clk.Now())
	}, hc)
	if err != nil {
		return nil, err
	}
	return appuser.NewBillingAccessService(client, conns, users, clk), nil
}
