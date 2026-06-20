// CQRS read side of the currency module. ReadService answers both currency
// endpoints by issuing purpose-built read queries and building the response DTOs
// directly. The module has no write side.
package currency

import (
	"context"

	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// ReadModel is the read-side data source. The infra currency ReadRepo implements
// it. Returning lightweight view rows keeps the read path free of aggregate
// hydration.
type ReadModel interface {
	// CurrencyListView returns all currencies ordered by code ASC.
	CurrencyListView(ctx context.Context) ([]CurrencyViewRow, error)
	// LatestCurrencyRateListView returns every rate on the most-recent published
	// date.
	LatestCurrencyRateListView(ctx context.Context) ([]CurrencyRateViewRow, error)
}

// CurrencyViewRow is the read-side currency row. Name is the raw (nullable) DB
// value, which is NULL in practice — the service resolves the wire name from the
// Intl display-name table, mirroring the PHP Currency::getName() fallback.
type CurrencyViewRow struct {
	ID             string
	Code           string
	Symbol         string
	Name           *string
	FractionDigits int16
}

// CurrencyRateViewRow is the read-side rate row. UpdatedAt arrives pre-formatted
// "Y-m-d 00:00:00" from the repo.
type CurrencyRateViewRow struct {
	CurrencyID     string
	BaseCurrencyID string
	Rate           string
	UpdatedAt      string
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
// fallback to the code when no entry exists — matching the PHP behaviour.
func (s *ReadService) GetCurrencyList(ctx context.Context, _ vo.Id) (*GetCurrencyListResult, error) {
	rows, err := s.read.CurrencyListView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]CurrencyResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, CurrencyResult{
			Id:             r.ID,
			Code:           r.Code,
			Name:           currencyName(r),
			Symbol:         r.Symbol,
			FractionDigits: int(r.FractionDigits),
		})
	}
	return &GetCurrencyListResult{Items: items}, nil
}

// currencyName resolves the wire display name: a non-empty stored name wins
// (PHP returns it directly), otherwise the Intl table by code (which itself
// falls back to the code). In the live data the stored name is always NULL, so
// this resolves via the Intl table.
func currencyName(r CurrencyViewRow) string {
	if r.Name != nil && *r.Name != "" {
		return *r.Name
	}
	return domcurrency.DisplayName(r.Code)
}

// GetCurrencyRateList returns the latest published rates, in the wire shape.
func (s *ReadService) GetCurrencyRateList(ctx context.Context, _ vo.Id) (*GetCurrencyRateListResult, error) {
	rows, err := s.read.LatestCurrencyRateListView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]CurrencyRateResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, CurrencyRateResult{
			CurrencyId:     r.CurrencyID,
			BaseCurrencyId: r.BaseCurrencyID,
			Rate:           r.Rate,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	return &GetCurrencyRateListResult{Items: items}, nil
}
