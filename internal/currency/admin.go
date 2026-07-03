// Write side of the currency module. The HTTP API exposes no currency
// mutations; these use cases exist only for the CLI admin commands
// (currency:update-rates, currency:add). The read side (read.go) is unchanged.
package currency

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// WriteModel is the write-side persistence port. The infra currency write repo
// implements it. All methods run on the context-bound querier, so the
// WriteService wraps multi-row work in a single transaction via its TxRunner.
type WriteModel interface {
	// CurrencyCodes returns a map of stored code -> currency id for every
	// currency.
	CurrencyCodes(ctx context.Context) (map[string]string, error)
	// CurrencyExists reports whether a currency with the (already-normalized)
	// code exists.
	CurrencyExists(ctx context.Context, code string) (bool, error)
	// InsertCurrency adds a new currency row.
	InsertCurrency(ctx context.Context, c CurrencyRow) error
	// UpsertRate inserts or updates a single (date, currency, base) rate.
	UpsertRate(ctx context.Context, r RateRow) error
}

// CurrencyRow is the data for a new currencies row.
type CurrencyRow struct {
	ID             string
	Code           string
	Symbol         string
	Name           *string
	FractionDigits int
	CreatedAt      time.Time
}

// RateRow is one currencies_rates row to upsert. Date is the published date
// (midnight); the repo stores it as a 'Y-m-d' DATE.
type RateRow struct {
	ID             string
	CurrencyID     string
	BaseCurrencyID string
	Date           time.Time
	Rate           string // decimal string
}

// RateInput is one loaded exchange rate (from the Open Exchange Rates loader),
// keyed by ISO codes. The WriteService resolves the codes to currency ids.
type RateInput struct {
	Code string
	Base string
	Rate string
	Date time.Time
}

// WriteTxRunner is the transaction boundary the write service owns
// (backend.TxManager satisfies it).
type WriteTxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// WriteClock supplies the current time (clock.Real in production).
type WriteClock interface {
	Now() time.Time
}

// WriteService is the currency write-use-case orchestrator.
type WriteService struct {
	write  WriteModel
	tx     WriteTxRunner
	clock  WriteClock
	nextID func() vo.Id
}

// NewWriteService wires the currency write service.
func NewWriteService(write WriteModel, tx WriteTxRunner, clock WriteClock) *WriteService {
	return &WriteService{write: write, tx: tx, clock: clock, nextID: vo.NewId}
}

// AvailableCodes returns every stored currency code (for the rate loader's
// `symbols` request). Order is unspecified.
func (s *WriteService) AvailableCodes(ctx context.Context) ([]string, error) {
	codes, err := s.write.CurrencyCodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(codes))
	for code := range codes {
		out = append(out, code)
	}
	// Sorted so the rate-loader's outbound `symbols` request and its debug log are
	// deterministic (the source map iteration order is not).
	sort.Strings(out)
	return out, nil
}

// UpdateRates upserts every loaded rate whose currency AND base code both resolve
// to a known currency, in one transaction. Unknown codes are skipped (not an
// error). Returns the number of rates actually written.
func (s *WriteService) UpdateRates(ctx context.Context, rates []RateInput) (int, error) {
	codes, err := s.write.CurrencyCodes(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, r := range rates {
			currencyID, ok := codes[normalizeCode(r.Code)]
			if !ok {
				continue
			}
			baseID, ok := codes[normalizeCode(r.Base)]
			if !ok {
				continue
			}
			if err := s.write.UpsertRate(ctx, RateRow{
				ID:             s.nextID().String(),
				CurrencyID:     currencyID,
				BaseCurrencyID: baseID,
				Date:           r.Date,
				Rate:           r.Rate,
			}); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// AddCurrency creates a currency if its code is not already present. The symbol
// and (when not overridden) the fraction digits come from the ICU tables.
// Returns whether a row was created (false = the code already existed).
func (s *WriteService) AddCurrency(ctx context.Context, code string, name *string, fractionDigits *int) (bool, error) {
	c, err := validateCode(code)
	if err != nil {
		return false, err
	}
	exists, err := s.write.CurrencyExists(ctx, c)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	digits := FractionDigits(c)
	if fractionDigits != nil {
		digits = *fractionDigits
	}
	row := CurrencyRow{
		ID:             s.nextID().String(),
		Code:           c,
		Symbol:         Symbol(c),
		Name:           name,
		FractionDigits: digits,
		CreatedAt:      s.clock.Now(),
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		return s.write.InsertCurrency(ctx, row)
	}); err != nil {
		return false, err
	}
	return true, nil
}

// normalizeCode upppercases and trims a currency code for map lookup. The stored
// codes are uppercase ISO 4217.
func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// validateCode normalizes and enforces the ISO 4217 shape (3 ASCII letters),
// returning a *ValidationError on failure.
func validateCode(code string) (string, error) {
	c := normalizeCode(code)
	if len(c) != 3 || !isAlpha(c) {
		return "", errs.NewValidation("CurrencyCode is incorrect",
			errs.FieldError{Key: "currency", Message: "CurrencyCode is incorrect"})
	}
	return c, nil
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
