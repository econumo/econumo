package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNormalizeAffiliate(t *testing.T) {
	cases := map[string]string{
		"cmwirmwzi0031h9pc8rhijzge": "cmwirmwzi0031h9pc8rhijzge",
		"a_b-C9":                    "a_b-C9",
		strings.Repeat("a", 64):     strings.Repeat("a", 64),
		strings.Repeat("a", 65):     "",
		"a b":                       "",
		"<x>":                       "",
		"":                          "",
	}
	for in, want := range cases {
		if got := model.NormalizeAffiliate(in); got != want {
			t.Errorf("NormalizeAffiliate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetAffiliateWriteOnce(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	u := model.NewUser(vo.NewId(), "a@example.test", "A", "face:fuchsia", "h", "s", now)

	u.SetAffiliate("", vo.NewId(), now)
	if u.Option(model.OptionAffiliate) != nil {
		t.Fatal("empty value added an option")
	}
	u.SetAffiliate("first", vo.NewId(), now)
	u.SetAffiliate("second", vo.NewId(), now)
	if got := u.Affiliate(); got != "first" {
		t.Fatalf("Affiliate() = %q, want first", got)
	}
	if !model.IsServerOnlyOption(model.OptionAffiliate) || model.IsServerOnlyOption(model.OptionAI) {
		t.Fatal("IsServerOnlyOption wrong")
	}
}
