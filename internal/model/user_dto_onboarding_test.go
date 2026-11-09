package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestUpdateOnboardingRequestValidate(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name     string
		value    *string
		wantCode string
	}{
		{"nil", nil, errs.CodeIsBlank},
		{"empty", str(""), errs.CodeIsBlank},
		{"started", str("started"), ""},
		{"completed", str("completed"), errs.CodeInvalidChoice},
		{"mixed", str("plan,!import"), ""},
		{"all", str("import,categories,!accounts,plan,!apple_wallet"), ""},
		{"duplicate", str("plan,plan"), errs.CodeInvalidChoice},
		{"duplicate skipped", str("plan,!plan"), errs.CodeInvalidChoice},
		{"unknown", str("foo"), errs.CodeInvalidChoice},
		{"started in list", str("started,plan"), errs.CodeInvalidChoice},
		{"empty item", str("plan,"), errs.CodeInvalidChoice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.UpdateOnboardingRequest{Value: tc.value}.Validate()
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			verr, ok := errs.AsValidation(err)
			if !ok || len(verr.Fields) != 1 || verr.Fields[0].Key != "value" || verr.Fields[0].Code != tc.wantCode {
				t.Fatalf("Validate() = %v, want value/%s", err, tc.wantCode)
			}
		})
	}
}
