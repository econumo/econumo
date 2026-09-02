package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestCreatePersonalTokenRequest_Scope(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		code  string
	}{
		{"full ok", "full", ""},
		{"ingest ok", "ingest", ""},
		{"blank", "", errs.CodeIsBlank},
		{"unknown", "admin", errs.CodeInvalidChoice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.CreatePersonalTokenRequest{Name: "ci", Scope: tc.scope}.Validate()
			if tc.code == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			ve, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("got %T, want *errs.ValidationError", err)
			}
			var found bool
			for _, f := range ve.Fields {
				if f.Key == "scope" && f.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("no scope field error with code %q in %+v", tc.code, ve.Fields)
			}
		})
	}
}
