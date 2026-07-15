// CQRS read side of the currency module. ReadService answers both currency
// endpoints by issuing purpose-built read queries and building the response DTOs
// directly. The module has no write side.
package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Scope values for CurrencyResult.Scope: "global" (no owner), "own" (the
// caller's custom currency), "shared" (a foreign custom reachable via a
// shared account/budget).
const (
	ScopeGlobal = "global"
	ScopeOwn    = "own"
	ScopeShared = "shared"
)

// ReadModel is the read-side data source. The infra currency ReadRepo implements
// it. Returning lightweight view rows keeps the read path free of aggregate
// hydration.
type ReadModel interface {
	// UserCurrencyListView returns every currency visible to userID: all
	// globals, the user's own customs, and foreign customs reachable via a
	// shared account/budget/budget-element.
	UserCurrencyListView(ctx context.Context, userID string) ([]model.CurrencyViewRow, error)
	// HiddenCurrencyIDs returns the ids of global currencies userID has hidden.
	HiddenCurrencyIDs(ctx context.Context, userID string) ([]string, error)
	// LatestCurrencyRateListView returns the latest rate row per (currency,
	// base) pair.
	LatestCurrencyRateListView(ctx context.Context) ([]model.CurrencyRateViewRow, error)
}

// ReadService serves both currency read endpoints.
type ReadService struct {
	read ReadModel
}

// NewReadService wires the read service.
func NewReadService(read ReadModel) *ReadService {
	return &ReadService{read: read}
}

// GetCurrencyList returns every currency visible to userID (globals + own +
// shared-reachable customs), ordered by code then id, in the wire shape. The
// display name comes from the Intl table (currencies.name is NULL), with a
// fallback to the code when no entry exists.
func (s *ReadService) GetCurrencyList(ctx context.Context, userID vo.Id) (*model.GetCurrencyListResult, error) {
	uid := userID.String()
	rows, err := s.read.UserCurrencyListView(ctx, uid)
	if err != nil {
		return nil, err
	}
	hidden, err := s.read.HiddenCurrencyIDs(ctx, uid)
	if err != nil {
		return nil, err
	}
	hiddenSet := make(map[string]bool, len(hidden))
	for _, id := range hidden {
		hiddenSet[id] = true
	}
	items := make([]model.CurrencyResult, 0, len(rows))
	for _, r := range rows {
		scope := ScopeGlobal
		if r.UserID != nil {
			if *r.UserID == uid {
				scope = ScopeOwn
			} else {
				scope = ScopeShared
			}
		}
		archived := 0
		if r.IsArchived {
			archived = 1
		}
		isHidden := 0
		if scope == ScopeGlobal && hiddenSet[r.ID] {
			isHidden = 1
		}
		items = append(items, model.CurrencyResult{
			Id:             r.ID,
			Code:           r.Code,
			Name:           currencyName(r),
			Symbol:         r.Symbol,
			FractionDigits: int(r.FractionDigits),
			Scope:          scope,
			IsArchived:     archived,
			IsHidden:       isHidden,
		})
	}
	return &model.GetCurrencyListResult{Items: items}, nil
}

// currencyName resolves the wire display name: a non-empty stored name wins,
// otherwise the Intl table by code (which itself falls back to the code). In the
// live data the stored name is always NULL, so this resolves via the Intl table.
func currencyName(r model.CurrencyViewRow) string {
	if r.Name != nil && *r.Name != "" {
		return *r.Name
	}
	return DisplayName(r.Code)
}

// GetCurrencyRateList returns the latest published rate per currency, filtered
// to currencies visible to userID, in the wire shape.
func (s *ReadService) GetCurrencyRateList(ctx context.Context, userID vo.Id) (*model.GetCurrencyRateListResult, error) {
	visible, err := s.read.UserCurrencyListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	visibleSet := make(map[string]bool, len(visible))
	for _, v := range visible {
		visibleSet[v.ID] = true
	}
	rows, err := s.read.LatestCurrencyRateListView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]model.CurrencyRateResult, 0, len(rows))
	for _, r := range rows {
		if !visibleSet[r.CurrencyID] {
			continue
		}
		items = append(items, model.CurrencyRateResult{
			CurrencyId:     r.CurrencyID,
			BaseCurrencyId: r.BaseCurrencyID,
			Rate:           r.Rate,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	return &model.GetCurrencyRateListResult{Items: items}, nil
}
