package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNormalizeAffiliateCode(t *testing.T) {
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
		if got := model.NormalizeAffiliateCode(in); got != want {
			t.Errorf("NormalizeAffiliateCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetAffiliateWriteOnce(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	u := model.NewUser(vo.NewId(), "a@example.test", "A", "face:fuchsia", "h", "s", now)

	u.SetAffiliate("", "x", vo.NewId, now)
	u.SetAffiliate("affonso", "", vo.NewId, now)
	if u.Option(model.OptionAffiliate) != nil || u.Option(model.OptionAffiliateCode) != nil {
		t.Fatal("an empty program or code added an option")
	}
	u.SetAffiliate("affonso", "first", vo.NewId, now)
	u.SetAffiliate("other", "second", vo.NewId, now)
	if u.Affiliate() != "affonso" || u.AffiliateCode() != "first" {
		t.Fatalf("got %q %q, want affonso first", u.Affiliate(), u.AffiliateCode())
	}
	if !model.IsServerOnlyOption(model.OptionAffiliate) || !model.IsServerOnlyOption(model.OptionAffiliateCode) || model.IsServerOnlyOption(model.OptionAI) {
		t.Fatal("IsServerOnlyOption wrong")
	}

	half := model.NewUser(vo.NewId(), "b@example.test", "B", "face:fuchsia", "h", "s", now)
	v := "old"
	half.Options = append(half.Options, model.NewUserOption(vo.NewId(), model.OptionAffiliateCode, &v, now))
	half.SetAffiliate("affonso", "new", vo.NewId, now)
	if half.Option(model.OptionAffiliate) != nil || half.AffiliateCode() != "old" {
		t.Fatal("either option present must write neither")
	}
}

func TestValidAffiliateProgram(t *testing.T) {
	for in, want := range map[string]bool{"affonso": true, "a_b-1": true, strings.Repeat("a", 32): true, strings.Repeat("a", 33): false, "A": false, "a b": false, "": false} {
		if got := model.ValidAffiliateProgram(in); got != want {
			t.Errorf("ValidAffiliateProgram(%q) = %v", in, got)
		}
	}
}
