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

// RatePeriod is the unit a rate-gap check works in. Conversions average each
// month's rates, so one rate per month is enough for them; RatePeriodDay asks
// for a rate on every day.
type RatePeriod int

const (
	RatePeriodMonth RatePeriod = iota
	RatePeriodDay
)

// RatedPeriod says a currency has at least one rate against the base inside
// the period starting at Start (the 1st of the month, or the day).
type RatedPeriod struct {
	Start time.Time
	Code  string
}

// RateGap is a period with no rate for some of the checked currencies.
type RateGap struct {
	Start   time.Time
	Missing []string
}

// RateGapReport is the result of a rate-gap check over [From, To], both
// period starts.
type RateGapReport struct {
	Base       string
	Period     RatePeriod
	From       time.Time
	To         time.Time
	Currencies []string
	Gaps       []RateGap
}

// RateBackfill is the outcome of loading the rates of one gap.
type RateBackfill struct {
	Date        time.Time
	Requested   []string
	Stored      int
	Unavailable []string
}

// CurrencyRecord is one currencies row with ownership and archival state.
// UserID nil = global (admin-managed); non-nil = custom, owned by that user.
type CurrencyRecord struct {
	ID             string
	Code           string
	Symbol         string
	Name           *string
	FractionDigits int
	UserID         *string
	// Rate is the fixed conversion rate for customs ("X per 1 base unit");
	// always nil for globals. NULL customs predate the mandatory-rate rule.
	Rate      *string
	CreatedAt time.Time
	// IsDeleted marks a soft-deleted custom currency: it stays resolvable for
	// the entities that still reference it, but leaves every picker and list.
	IsDeleted bool
}

// BaseCurrency is the instance base currency, resolved at boot: every stored
// rate is denominated "X units per 1 base unit". Constructing one is the
// composition root's job, so consumers can trust ID references an existing
// global currency row.
type BaseCurrency struct {
	ID   string
	Code string
}
