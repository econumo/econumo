package imports

import (
	"context"
	"errors"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ClaimSetupToken exchanges a one-time SimpleFIN setup token for the access
// URL and hands it straight back: the browser encrypts it, the server never
// keeps it.
func (s *Service) ClaimSetupToken(ctx context.Context, userID vo.Id, req model.ClaimSetupTokenRequest) (*model.ClaimSetupTokenResult, error) {
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeClaimSetupToken, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeClaimSetupToken, userID.String())
	}
	c, ok := s.claimer(model.ImportProviderSimpleFIN)
	if !ok {
		return nil, &errs.ValidationError{Msg: "Provider is not supported", MsgCode: errs.CodeImportProviderUnsupported}
	}
	accessURL, err := c.ClaimSetupToken(ctx, strings.TrimSpace(req.SetupToken))
	if err != nil {
		return nil, mapProviderErr(err)
	}
	return &model.ClaimSetupTokenResult{AccessUrl: accessURL}, nil
}

// mapProviderErr turns the provider sentinels into coded edge errors. Any
// other error is passed through unchanged (500 at the edge) — it never
// carries the credential because the client never formats it into an error.
func mapProviderErr(err error) error {
	switch {
	case errors.Is(err, ErrSetupTokenRejected):
		return &errs.AccessDeniedError{Msg: "Setup token rejected", Code: errs.CodeImportSetupTokenRejected}
	case errors.Is(err, ErrCredentialInvalid):
		return &errs.ValidationError{Msg: "Access URL is invalid", MsgCode: errs.CodeImportAccessUrlInvalid}
	case errors.Is(err, ErrProviderUnavailable):
		return &errs.ValidationError{Msg: "Provider unavailable", MsgCode: errs.CodeImportProviderUnavailable}
	}
	return err
}

func (s *Service) GetCredentialKey(ctx context.Context, userID vo.Id) (*model.GetImportCredentialKeyResult, error) {
	k, err := s.repo.GetCredentialKey(ctx, userID)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, errs.NewNotFound("Credential key not found")
	}
	return credentialKeyResult(k), nil
}

func (s *Service) SetCredentialKey(ctx context.Context, userID vo.Id, req model.SetImportCredentialKeyRequest) (*model.GetImportCredentialKeyResult, error) {
	now := s.clk.Now().UTC()
	k := &model.ImportCredentialKey{UserID: userID, WrappedDataKey: req.WrappedDataKey, KDF: req.Kdf, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.UpsertCredentialKey(ctx, k); err != nil {
		return nil, err
	}
	return credentialKeyResult(k), nil
}

func credentialKeyResult(k *model.ImportCredentialKey) *model.GetImportCredentialKeyResult {
	return &model.GetImportCredentialKeyResult{WrappedDataKey: k.WrappedDataKey, Kdf: k.KDF, UpdatedAt: k.UpdatedAt.Format(datetime.Layout)}
}

// pullSource is ownedSource plus the pull-provider checks every credential
// call shares.
func (s *Service) pullSource(ctx context.Context, userID vo.Id, rawID string) (*model.ImportSource, Provider, error) {
	src, err := s.ownedSource(ctx, userID, rawID)
	if err != nil {
		return nil, nil, err
	}
	p, ok := s.provider(src.Provider)
	if model.ImportProviderIsPush(src.Provider) || !ok {
		return nil, nil, &errs.ValidationError{Msg: "Provider is not supported", MsgCode: errs.CodeImportProviderUnsupported}
	}
	return src, p, nil
}

func (s *Service) ListExternalAccounts(ctx context.Context, userID vo.Id, req model.ListExternalAccountsRequest) (*model.ListExternalAccountsResult, error) {
	src, p, err := s.pullSource(ctx, userID, req.SourceId)
	if err != nil {
		return nil, err
	}
	accounts, err := p.ListAccounts(ctx, Credential{AccessURL: req.AccessUrl})
	if err != nil {
		return nil, mapProviderErr(err)
	}
	items, err := s.externalAccountResults(ctx, src, accounts)
	if err != nil {
		return nil, err
	}
	return &model.ListExternalAccountsResult{Items: items}, nil
}

// externalAccountResults joins what the bridge reports with the user's
// mapping state for each account.
func (s *Service) externalAccountResults(ctx context.Context, src *model.ImportSource, accounts []model.ExternalAccount) ([]model.ExternalAccountResult, error) {
	links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ExternalAccountResult, 0, len(accounts))
	for _, a := range accounts {
		item := model.ExternalAccountResult{
			ExternalAccountId: a.ID, ExternalName: a.Name, ExternalCurrency: a.Currency, Balance: a.Balance, OrgName: a.OrgName,
			State: model.ImportCardStateUnmapped,
		}
		if al := findAccountLink(links, a.ID); al != nil {
			item.State = model.ImportCardStateMapped
			if al.Mode == model.ImportAccountLinkModeIgnore {
				item.State = model.ImportCardStateIgnored
			}
			item.AccountId = idString(al.AccountID)
		}
		out = append(out, item)
	}
	return out, nil
}
