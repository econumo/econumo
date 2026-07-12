package model

import (
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

func fieldMessage(t *testing.T, err error, key string) string {
	t.Helper()
	if err == nil {
		t.Fatal("want validation error, got nil")
	}
	verr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("err type %T, want *errs.ValidationError", err)
	}
	for _, f := range verr.Fields {
		if f.Key == key {
			return f.Message
		}
	}
	t.Fatalf("no field error for %q in %v", key, verr.Fields)
	return ""
}

func TestTransactionListRequest_Validate_Paging(t *testing.T) {
	const acct = "a0000000-0000-0000-0000-000000000001"

	cases := []struct {
		name    string
		req     TransactionListRequest
		key     string
		wantMsg string
	}{
		{"limit not a number", TransactionListRequest{AccountId: acct, Limit: "abc"},
			"limit", "This value should be an integer between 1 and 500."},
		{"limit zero", TransactionListRequest{AccountId: acct, Limit: "0"},
			"limit", "This value should be an integer between 1 and 500."},
		{"limit too big", TransactionListRequest{AccountId: acct, Limit: "501"},
			"limit", "This value should be an integer between 1 and 500."},
		{"limit without accountId", TransactionListRequest{Limit: "50"},
			"limit", "limit requires accountId."},
		{"limit with period", TransactionListRequest{AccountId: acct, Limit: "50",
			PeriodStart: "2026-01-01 00:00:00", PeriodEnd: "2026-02-01 00:00:00"},
			"limit", "limit cannot be combined with periodStart or periodEnd."},
		{"cursor without limit", TransactionListRequest{AccountId: acct, Cursor: "abc"},
			"cursor", "cursor requires limit."},
		{"perAccountLimit bad", TransactionListRequest{PerAccountLimit: "-1"},
			"perAccountLimit", "This value should be an integer between 1 and 500."},
		{"perAccountLimit combined", TransactionListRequest{PerAccountLimit: "50", AccountId: acct},
			"perAccountLimit", "perAccountLimit cannot be combined with other parameters."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldMessage(t, tc.req.Validate(), tc.key); got != tc.wantMsg {
				t.Errorf("message = %q, want %q", got, tc.wantMsg)
			}
		})
	}

	valid := []TransactionListRequest{
		{},
		{AccountId: acct},
		{AccountId: acct, Limit: "50"},
		{AccountId: acct, Limit: "1", Cursor: "whatever"}, // cursor CONTENT is checked in the use case
		{PerAccountLimit: "500"},
	}
	for _, req := range valid {
		if err := req.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", req, err)
		}
	}
}
