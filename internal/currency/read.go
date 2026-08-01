// CQRS read side of the currency module. ReadService answers both currency
// endpoints by issuing purpose-built read queries and building the response DTOs
// directly. The module has no write side.
package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
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

// ReadService serves both currency read endpoints. base is the boot-resolved
// instance base currency, needed to synthesize the base's own rate row in
// GetCurrencyRateList.
type ReadService struct {
	read ReadModel
	base model.BaseCurrency
}

// NewReadService wires the read service.
func NewReadService(read ReadModel, base model.BaseCurrency) *ReadService {
	return &ReadService{read: read, base: base}
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
	items := make([]model.CurrencyListItem, 0, len(rows))
	for _, r := range rows {
		scope := ScopeGlobal
		if r.UserID != nil {
			if *r.UserID == uid {
				scope = ScopeOwn
			} else {
				scope = ScopeShared
			}
		}
		isHidden := 0
		if scope != ScopeShared && hiddenSet[r.ID] {
			isHidden = 1
		}
		items = append(items, model.CurrencyListItem{
			CurrencyResult: model.CurrencyResult{
				Id:             r.ID,
				Code:           r.Code,
				Name:           currencyName(r),
				Symbol:         r.Symbol,
				FractionDigits: int(r.FractionDigits),
			},
			Scope:    scope,
			IsHidden: isHidden,
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
//
// Rates are stored as X-per-base, so the base currency has no stored row
// against itself unless a rate feed wrote one (OXR does, at 1.0). Clients
// convert through the base and treat a missing rate row as "fall back to
// 1:1", which silently breaks every base<->custom conversion on instances
// without such a feed — so when rates exist but the base has none, a
// synthetic base/base = 1 row is appended, dated like the newest real rate.
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
	baseHasRate := false
	latest := ""
	for _, r := range rows {
		if !visibleSet[r.CurrencyID] {
			continue
		}
		if r.CurrencyID == s.base.ID {
			baseHasRate = true
		}
		if r.UpdatedAt > latest {
			latest = r.UpdatedAt
		}
		items = append(items, model.CurrencyRateResult{
			CurrencyId:     r.CurrencyID,
			BaseCurrencyId: r.BaseCurrencyID,
			Rate:           r.Rate,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	// Custom currencies carry ONE fixed rate on the currency row (no dated
	// history); serving it here keeps this endpoint every client's single
	// conversion source. updatedAt is the currency's creation time — cosmetic,
	// since a fixed rate has no publication date. NULL-rate legacy customs
	// (created before the rate became mandatory) yield nothing.
	for _, v := range visible {
		if v.UserID == nil || v.Rate == nil {
			continue
		}
		updatedAt := v.CreatedAt.Format(datetime.Layout)
		if updatedAt > latest {
			latest = updatedAt
		}
		items = append(items, model.CurrencyRateResult{
			CurrencyId:     v.ID,
			BaseCurrencyId: s.base.ID,
			Rate:           vo.NewDecimal(*v.Rate).String(),
			UpdatedAt:      updatedAt,
		})
	}
	if !baseHasRate && len(items) > 0 {
		items = append(items, model.CurrencyRateResult{
			CurrencyId:     s.base.ID,
			BaseCurrencyId: s.base.ID,
			Rate:           "1",
			UpdatedAt:      latest,
		})
	}
	return &model.GetCurrencyRateListResult{Items: items}, nil
}
