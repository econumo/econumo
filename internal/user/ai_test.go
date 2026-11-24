package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type recordingClearer struct {
	cleared []vo.Id
	err     error
}

func (c *recordingClearer) ClearSuggestions(_ context.Context, userID vo.Id) error {
	c.cleared = append(c.cleared, userID)
	return c.err
}

func TestUpdateAI_TurningOffClearsSuggestions(t *testing.T) {
	svc, _, _, _, _, _, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "Me", "me@econumo.test", "secretpass1")
	clearer := &recordingClearer{}
	svc.SetAISuggestionClearer(clearer)
	off, on := false, true

	if _, err := svc.UpdateAI(ctx, uid, model.UpdateAIRequest{Enabled: &on}); err != nil || len(clearer.cleared) != 0 {
		t.Fatalf("turning on: err = %v, cleared = %v", err, clearer.cleared)
	}
	if _, err := svc.UpdateAI(ctx, uid, model.UpdateAIRequest{Enabled: &off}); err != nil || len(clearer.cleared) != 1 || clearer.cleared[0] != uid {
		t.Fatalf("turning off: err = %v, cleared = %v", err, clearer.cleared)
	}
}

func TestUpdateAI_ClearerFailureNeverFailsTheToggle(t *testing.T) {
	svc, _, _, _, _, _, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "Me", "me@econumo.test", "secretpass1")
	svc.SetAISuggestionClearer(&recordingClearer{err: errors.New("boom")})
	off := false
	res, err := svc.UpdateAI(ctx, uid, model.UpdateAIRequest{Enabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range res.User.Options {
		if o.Name == model.OptionAI && o.Value != nil && *o.Value == "0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("options = %+v", res.User.Options)
	}
}

func TestUpdateAISuggestions(t *testing.T) {
	svc, _, _, _, _, _, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "Me", "me@econumo.test", "secretpass1")
	rules, fill := false, true
	res, err := svc.UpdateAISuggestions(ctx, uid, model.UpdateAISuggestionsRequest{SuggestRules: &rules, SuggestFill: &fill})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, o := range res.User.Options {
		if o.Value != nil {
			got[o.Name] = *o.Value
		}
	}
	if got[model.OptionAISuggestRules] != "0" || got[model.OptionAISuggestFill] != "1" {
		t.Fatalf("options = %+v", res.User.Options)
	}
	ve, ok := errs.AsValidation(model.UpdateAISuggestionsRequest{SuggestRules: &rules}.Validate())
	if !ok || len(ve.Fields) != 1 || ve.Fields[0].Key != "suggestFill" {
		t.Fatalf("validate = %+v", ve)
	}
}
