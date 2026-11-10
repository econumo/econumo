package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func TestMarkOnboardingStep(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	s := func(v string) *string { return &v }
	cases := []struct {
		stored *string
		want   *string
	}{
		{s("started"), s("categories")},
		{s("!accounts,!categories"), s("!accounts,categories")},
		{s("plan,categories"), s("plan,categories")},
		{s("completed"), s("completed")},
		{nil, nil},
	}
	for _, c := range cases {
		u := &User{ID: vo.NewId()}
		if c.stored != nil {
			u.Options = append(u.Options, NewUserOption(vo.NewId(), OptionOnboarding, c.stored, now))
		}
		u.MarkOnboardingStep("categories", now)
		got := u.Option(OptionOnboarding)
		switch {
		case c.want == nil && got != nil:
			t.Fatalf("%v: option appeared", c.stored)
		case c.want != nil && (got == nil || *got.Value != *c.want):
			t.Fatalf("%v: got %v, want %s", *c.stored, got, *c.want)
		}
	}
	if OnboardingStepDone(s("!categories"), "categories") || !OnboardingStepDone(s("accounts,categories"), "categories") || OnboardingStepDone(s("started"), "plan") {
		t.Fatal("OnboardingStepDone")
	}
}
