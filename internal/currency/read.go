// CQRS read side of the currency module. ReadService answers both currency
// endpoints by issuing purpose-built read queries and building the response DTOs
// directly. The module has no write side.
package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the read-side data source. The infra currency ReadRepo implements
// it. Returning lightweight view rows keeps the read path free of aggregate
// hydration.
type ReadModel interface {
	// CurrencyListView returns all currencies ordered by code ASC.
	CurrencyListView(ctx context.Context) ([]model.CurrencyViewRow, error)
	// LatestCurrencyRateListView returns every rate on the most-recent published
	// date.
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

// GetCurrencyList returns all currencies ordered by code, in the wire shape.
// The display name comes from the Intl table (currencies.name is NULL), with a
// fallback to the code when no entry exists.
func (s *ReadService) GetCurrencyList(ctx context.Context, _ vo.Id) (*model.GetCurrencyListResult, error) {
	rows, err := s.read.CurrencyListView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]model.CurrencyResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, model.CurrencyResult{
			Id:             r.ID,
			Code:           r.Code,
			Name:           currencyName(r),
			Symbol:         r.Symbol,
			FractionDigits: int(r.FractionDigits),
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

// GetCurrencyRateList returns the latest published rates, in the wire shape.
func (s *ReadService) GetCurrencyRateList(ctx context.Context, _ vo.Id) (*model.GetCurrencyRateListResult, error) {
	rows, err := s.read.LatestCurrencyRateListView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]model.CurrencyRateResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, model.CurrencyRateResult{
			CurrencyId:     r.CurrencyID,
			BaseCurrencyId: r.BaseCurrencyID,
			Rate:           r.Rate,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	return &model.GetCurrencyRateListResult{Items: items}, nil
}
