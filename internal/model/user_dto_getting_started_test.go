package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestUpdateGettingStartedRequestValidate(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name     string
		value    *string
		wantCode string
	}{
		{"nil", nil, errs.CodeIsBlank},
		{"mixed", str("plan,!import"), ""},
		{"empty", str(""), ""},
		{"all", str("plan,import,!apple_wallet"), ""},
		{"new steps", str("import,categories,!accounts,plan"), ""},
		{"duplicate new", str("categories,categories"), errs.CodeInvalidChoice},
		{"duplicate", str("plan,plan"), errs.CodeInvalidChoice},
		{"duplicate dismissed", str("plan,!plan"), errs.CodeInvalidChoice},
		{"unknown", str("foo"), errs.CodeInvalidChoice},
		{"empty item", str("plan,"), errs.CodeInvalidChoice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.UpdateGettingStartedRequest{Value: tc.value}.Validate()
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
