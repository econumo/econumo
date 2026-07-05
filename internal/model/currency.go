package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// FullRate is one currency's average rate for a period: rate units of the
// currency per one base unit.
type FullRate struct {
	CurrencyID vo.Id
	Rate       vo.DecimalNumber
}

// ConvertItem is one bulk-conversion request: convert `Amount` from From to To,
// using the rate period containing PeriodStart.
type ConvertItem struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	From        vo.Id
	To          vo.Id
	Amount      vo.DecimalNumber
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
// keyed by ISO codes. The currency WriteService resolves the codes to
// currency ids.
type RateInput struct {
	Code string
	Base string
	Rate string
	Date time.Time
}
