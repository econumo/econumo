package imports

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
)

// Credential is a pull provider's secret for one call. It arrives from the
// client per request (decrypted in the browser) and is never persisted or
// logged by the server.
type Credential struct {
	AccessURL string
}

type FetchRequest struct {
	StartDate, EndDate time.Time
}

// FetchResult carries both halves of one provider round trip: SimpleFIN
// returns accounts and their transactions in a single document, so one call
// serves account discovery and the sync.
type FetchResult struct {
	Accounts     []model.ExternalAccount
	Transactions []model.ExternalTransaction
	Warnings     []string
}

type Provider interface {
	ListAccounts(ctx context.Context, cred Credential) ([]model.ExternalAccount, error)
	FetchTransactions(ctx context.Context, cred Credential, req FetchRequest) (*FetchResult, error)
}

// SetupTokenClaimer is the optional one-time exchange a provider offers
// (SimpleFIN: setup token -> access URL). The result goes back to the caller
// and is forgotten.
type SetupTokenClaimer interface {
	ClaimSetupToken(ctx context.Context, setupToken string) (string, error)
}

var (
	ErrSetupTokenRejected  = errors.New("setup token rejected")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrCredentialInvalid   = errors.New("credential invalid")
)

func (s *Service) RegisterProvider(name string, p Provider) {
	if s.providers == nil {
		s.providers = map[string]Provider{}
	}
	s.providers[name] = p
}

func (s *Service) provider(name string) (Provider, bool) {
	p, ok := s.providers[name]
	return p, ok
}

func (s *Service) claimer(name string) (SetupTokenClaimer, bool) {
	c, ok := s.providers[name].(SetupTokenClaimer)
	return c, ok
}
