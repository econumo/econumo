package model_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func TestUpdateAvatarRequestValidate(t *testing.T) {
	cases := []struct {
		name       string
		req        model.UpdateAvatarRequest
		wantFields map[string]string // key -> code
	}{
		{"valid", model.UpdateAvatarRequest{Icon: "face", Color: "fuchsia"}, nil},
		{"both blank", model.UpdateAvatarRequest{}, map[string]string{"icon": errs.CodeIsBlank, "color": errs.CodeIsBlank}},
		{"blank icon", model.UpdateAvatarRequest{Color: "red"}, map[string]string{"icon": errs.CodeIsBlank}},
		{"blank color", model.UpdateAvatarRequest{Icon: "face"}, map[string]string{"color": errs.CodeIsBlank}},
		{"whitespace icon", model.UpdateAvatarRequest{Icon: "   ", Color: "red"}, map[string]string{"icon": errs.CodeIsBlank}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantFields == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			verr, ok := errs.AsValidation(err)
			if !ok {
				t.Fatalf("Validate() = %v, want *ValidationError", err)
			}
			if len(verr.Fields) != len(tc.wantFields) {
				t.Fatalf("got %d field errors %v, want %d", len(verr.Fields), verr.Fields, len(tc.wantFields))
			}
			for _, f := range verr.Fields {
				if code, ok := tc.wantFields[f.Key]; !ok || f.Code != code {
					t.Errorf("unexpected field error %+v", f)
				}
			}
		})
	}
}
