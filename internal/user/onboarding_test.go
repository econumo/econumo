package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func onboardingValue(t *testing.T, opts []model.OptionResult) string {
	t.Helper()
	for _, o := range opts {
		if o.Name == model.OptionOnboarding && o.Value != nil {
			return *o.Value
		}
	}
	t.Fatalf("onboarding missing from options: %+v", opts)
	return ""
}

func TestUpdateOnboardingPersistsUntilCompleted(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Onboarding", "onb@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	v := "plan,!import"
	res, err := svc.UpdateOnboarding(ctx, id, model.UpdateOnboardingRequest{Value: &v})
	if err != nil {
		t.Fatalf("UpdateOnboarding: %v", err)
	}
	if got := onboardingValue(t, res.User.Options); got != v {
		t.Fatalf("onboarding = %q, want %q", got, v)
	}

	if _, err := svc.CompleteOnboarding(ctx, id); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	late := "plan"
	res, err = svc.UpdateOnboarding(ctx, id, model.UpdateOnboardingRequest{Value: &late})
	if err != nil {
		t.Fatalf("UpdateOnboarding after completion: %v", err)
	}
	if got := onboardingValue(t, res.User.Options); got != model.OnboardingCompleted {
		t.Fatalf("onboarding = %q, want completed", got)
	}
}
