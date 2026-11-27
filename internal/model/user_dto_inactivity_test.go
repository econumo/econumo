package model_test

import (
	"encoding/json"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestUpdateInactivityPeriodRequestValidate(t *testing.T) {
	for _, c := range []struct {
		body string
		want *int // nil = refused; whether the value is a choice is the service's check
	}{
		{`{"months": 3}`, ptr(3)},
		{`{"months": 24}`, ptr(24)},
		{`{"months": 0}`, ptr(0)},
		{`{"months": -6}`, ptr(-6)},
		{`{}`, nil},
		{`{"months": null}`, nil},
		{`{"months": 6.5}`, nil},
		{`{"months": 6.0}`, nil},
		{`{"months": 6e0}`, nil},
		{`{"months": "6"}`, nil},
		{`{"months": true}`, nil},
		{`{"months": [6]}`, nil},
	} {
		var req model.UpdateInactivityPeriodRequest
		if err := json.Unmarshal([]byte(c.body), &req); err != nil {
			t.Fatalf("%s: decode: %v", c.body, err)
		}
		err := req.Validate()
		if c.want != nil {
			if err != nil {
				t.Errorf("%s: Validate() = %v, want nil", c.body, err)
				continue
			}
			if got := req.MonthsValue(); got != *c.want {
				t.Errorf("%s: MonthsValue() = %d, want %d", c.body, got, *c.want)
			}
			continue
		}
		verr, ok := errs.AsValidation(err)
		if !ok || len(verr.Fields) != 1 || verr.Fields[0].Key != "months" || verr.Fields[0].Message != "This value is not valid." || verr.Fields[0].Code != errs.CodeInvalidFormat {
			t.Errorf("%s: Validate() = %#v, want the months error", c.body, err)
		}
	}
}

func ptr(n int) *int { return &n }
