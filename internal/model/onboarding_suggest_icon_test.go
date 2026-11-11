package model_test

import (
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestOnboardingSuggestIconRequestValidate(t *testing.T) {
	cases := []struct {
		name              string
		req               model.OnboardingSuggestIconRequest
		wantKey, wantCode string
	}{
		{"category", model.OnboardingSuggestIconRequest{Kind: "category", Name: "Rent", Type: "expense"}, "", ""},
		{"category without type", model.OnboardingSuggestIconRequest{Kind: "category", Name: "Rent"}, "", ""},
		{"account", model.OnboardingSuggestIconRequest{Kind: "account", Name: "Revolut", Currency: "eur"}, "", ""},
		{"one letter", model.OnboardingSuggestIconRequest{Kind: "account", Name: " x "}, "", ""},
		{"64 runes", model.OnboardingSuggestIconRequest{Kind: "account", Name: strings.Repeat("ж", 64)}, "", ""},
		{"no kind", model.OnboardingSuggestIconRequest{Name: "Rent"}, "kind", errs.CodeIsBlank},
		{"unknown kind", model.OnboardingSuggestIconRequest{Kind: "payee", Name: "Rent"}, "kind", errs.CodeInvalidChoice},
		{"blank name", model.OnboardingSuggestIconRequest{Kind: "category", Name: "   "}, "name", errs.CodeIsBlank},
		{"65 runes", model.OnboardingSuggestIconRequest{Kind: "category", Name: strings.Repeat("ж", 65)}, "name", errs.CodeTooLong},
		{"unknown type", model.OnboardingSuggestIconRequest{Kind: "category", Name: "Rent", Type: "transfer"}, "type", errs.CodeInvalidChoice},
		{"bad currency", model.OnboardingSuggestIconRequest{Kind: "account", Name: "Cash", Currency: "euro!"}, "currency", errs.CodeInvalidFormat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			verr, ok := errs.AsValidation(err)
			if !ok || len(verr.Fields) != 1 || verr.Fields[0].Key != tc.wantKey || verr.Fields[0].Code != tc.wantCode {
				t.Fatalf("Validate() = %v, want %s/%s", err, tc.wantKey, tc.wantCode)
			}
		})
	}
}
