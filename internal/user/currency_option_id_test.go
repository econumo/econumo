package user_test

// The profile currency option STORES a currency id; the wire keeps showing
// the code (frozen contract) and currency_id is the stored truth.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const currencyOptUSD = "dffc2a06-6f29-4704-8575-31709adee926"

func storedCurrencyOption(t *testing.T, db *dbtest.DB, userID string) string {
	t.Helper()
	var v string
	if err := db.Raw.QueryRow(db.Rebind(`SELECT value FROM users_options WHERE user_id = ? AND name = 'currency'`), userID).Scan(&v); err != nil {
		t.Fatalf("read stored currency option: %v", err)
	}
	return v
}

func optionValue(t *testing.T, opts []model.OptionResult, name string) string {
	t.Helper()
	for _, o := range opts {
		if o.Name == name {
			if o.Value == nil {
				return ""
			}
			return *o.Value
		}
	}
	t.Fatalf("option %q missing", name)
	return ""
}

func TestProfileCurrency_StoredAsID_WireShowsCode(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newTrialSvc(t, db, 0)
	ctx := context.Background()

	res, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Ida", Email: "ida@econumo.test", Password: "secretpass",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Registration seeds the RESOLVED default id, not the code.
	if got := storedCurrencyOption(t, db, res.User.Id); got != currencyOptUSD {
		t.Fatalf("stored option = %q, want the USD id %s", got, currencyOptUSD)
	}
	// The wire is frozen: code in the option + deprecated field; id separately.
	if res.User.Currency != "USD" {
		t.Fatalf("wire currency = %q, want USD", res.User.Currency)
	}
	if got := optionValue(t, res.User.Options, model.OptionCurrency); got != "USD" {
		t.Fatalf("wire option value = %q, want the CODE USD", got)
	}
	if got := optionValue(t, res.User.Options, model.OptionCurrencyID); got != currencyOptUSD {
		t.Fatalf("wire currency_id = %q, want %s", got, currencyOptUSD)
	}

	uid, err := vo.ParseId(res.User.Id)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture.New(t, db)
	pts := f.Currency(fixture.Currency{Code: "PTS", Symbol: "p", UserID: res.User.Id, Rate: "10.00000000"})

	upd, err := svc.UpdateCurrency(ctx, uid, model.UpdateCurrencyRequest{Currency: "PTS"})
	if err != nil {
		t.Fatalf("UpdateCurrency: %v", err)
	}
	if got := storedCurrencyOption(t, db, res.User.Id); got != pts {
		t.Fatalf("stored option = %q, want the custom's id %s", got, pts)
	}
	if upd.User.Currency != "PTS" {
		t.Fatalf("wire currency = %q, want PTS", upd.User.Currency)
	}
	if got := optionValue(t, upd.User.Options, model.OptionCurrency); got != "PTS" {
		t.Fatalf("wire option value = %q, want the CODE PTS", got)
	}
	if got := optionValue(t, upd.User.Options, model.OptionCurrencyID); got != pts {
		t.Fatalf("wire currency_id = %q, want %s", got, pts)
	}
}
