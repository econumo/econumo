package model

import (
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

func TestCurrencyRateListRequest_Validate(t *testing.T) {
	cases := []struct {
		name     string
		req      CurrencyRateListRequest
		wantKey  string
		wantCode string
		wantMsg  string
	}{
		{"neither", CurrencyRateListRequest{}, "", "", ""},
		{"from only", CurrencyRateListRequest{From: "2026-03-01", HasFrom: true}, "to", errs.CodeIsBlank, "This value should not be blank."},
		{"to only", CurrencyRateListRequest{To: "2026-03-01", HasTo: true}, "from", errs.CodeIsBlank, "This value should not be blank."},
		{"bad month", CurrencyRateListRequest{From: "2026-13-01", To: "2026-03-01", HasFrom: true, HasTo: true}, "from", errs.CodeInvalidFormat, "This value is not valid."},
		{"not padded", CurrencyRateListRequest{From: "2026-1-5", To: "2026-03-01", HasFrom: true, HasTo: true}, "from", errs.CodeInvalidFormat, "This value is not valid."},
		{"reversed", CurrencyRateListRequest{From: "2026-03-02", To: "2026-03-01", HasFrom: true, HasTo: true}, "to", errs.CodeCurrencyRateRangeOrder, "The end date must not be before the start date"},
		{"366 leap year", CurrencyRateListRequest{From: "2024-01-01", To: "2024-12-31", HasFrom: true, HasTo: true}, "", "", ""},
		{"366 across years", CurrencyRateListRequest{From: "2023-01-01", To: "2024-01-01", HasFrom: true, HasTo: true}, "", "", ""},
		{"367", CurrencyRateListRequest{From: "2023-01-01", To: "2024-01-02", HasFrom: true, HasTo: true}, "to", errs.CodeCurrencyRateRangeTooLong, "The date range must be at most 366 days"},
		{"same day", CurrencyRateListRequest{From: "2026-03-01", To: "2026-03-01", HasFrom: true, HasTo: true}, "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.req.Validate()
			if c.wantKey == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) || len(ve.Fields) != 1 {
				t.Fatalf("err = %v, want one field error", err)
			}
			f := ve.Fields[0]
			if f.Key != c.wantKey || f.Code != c.wantCode || f.Message != c.wantMsg {
				t.Fatalf("field = %+v, want %s/%s/%q", f, c.wantKey, c.wantCode, c.wantMsg)
			}
		})
	}
}

func TestCurrencyRateListRequest_Range(t *testing.T) {
	if _, _, ok := (CurrencyRateListRequest{}).Range(); ok {
		t.Fatal("empty request must not be ranged")
	}
	from, to, ok := CurrencyRateListRequest{From: "2026-03-01", To: "2026-03-31", HasFrom: true, HasTo: true}.Range()
	if !ok || from.Format("2006-01-02") != "2026-03-01" || to.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("range = %v %v %v", from, to, ok)
	}
}
