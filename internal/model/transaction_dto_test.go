package model

import (
	"strings"
	"testing"
)

func TestAccountTransactionListRequest_IDs(t *testing.T) {
	const a = "aaaa0000-0000-4000-8000-000000000001"
	const b = "aaaa0000-0000-4000-8000-000000000002"

	if ids := (AccountTransactionListRequest{}).IDs(); ids != nil {
		t.Fatalf("absent parameter: IDs = %v, want nil", ids)
	}
	got := AccountTransactionListRequest{AccountIds: a + ", " + b + "," + strings.ToUpper(a), Provided: true}.IDs()
	if len(got) != 2 || got[0].String() != a || got[1].String() != b {
		t.Fatalf("IDs = %v, want [%s %s] deduplicated in request order", got, a, b)
	}
}

func TestAccountTransactionListRequest_Validate(t *testing.T) {
	const a = "aaaa0000-0000-4000-8000-000000000001"
	cases := []struct {
		name    string
		req     AccountTransactionListRequest
		wantErr bool
	}{
		{"absent", AccountTransactionListRequest{}, false},
		{"one id", AccountTransactionListRequest{AccountIds: a, Provided: true}, false},
		{"empty", AccountTransactionListRequest{Provided: true}, true},
		{"whitespace and commas", AccountTransactionListRequest{AccountIds: " , ,", Provided: true}, true},
		{"not a uuid", AccountTransactionListRequest{AccountIds: "nope", Provided: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.req.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
