package budget_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

const (
	eurID     = "eeee1111-0000-7000-8000-000000000e12"
	eurAcctID = "aaaa2222-0000-7000-8000-000000000002"
	eurTxID   = "ffff1111-0000-7000-8000-000000000001"
)

// seedTwoCurrency adds a EUR currency + a Jan-2026 EUR rate, a EUR account owned
// by the seed user, and a 100.00 EUR expense in Jan 2026 categorized to the
// seeded Food category. Mirrors the convertor_provider seeding pattern.
func (h *harness) seedTwoCurrency(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1690000000, 0).UTC()
	// EUR currency (the baseline already inserted USD with usdID).
	if _, err := h.db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'EUR', 'E', 2, ?)`, eurID, now); err != nil {
		t.Fatalf("seed EUR: %v", err)
	}
	// Two EUR->USD rates in Jan 2026: AVG(0.90, 0.92) = 0.91.
	for _, r := range []struct{ id, date, rate string }{
		{"20000000-0000-7000-8000-000000000001", "2026-01-10", "0.90"},
		{"20000000-0000-7000-8000-000000000002", "2026-01-20", "0.92"},
	} {
		if _, err := h.db.ExecContext(ctx, `INSERT INTO currencies_rates (id, currency_id, base_currency_id, rate, published_at) VALUES (?, ?, ?, ?, ?)`,
			r.id, eurID, usdID, r.rate, r.date); err != nil {
			t.Fatalf("seed rate: %v", err)
		}
	}
	// EUR account owned by the seed user, in the Main folder.
	if _, err := h.db.ExecContext(ctx, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Euro', 2, 'wallet', 0, ?, ?)`,
		eurAcctID, eurID, seedUserID, now, now); err != nil {
		t.Fatalf("seed EUR account: %v", err)
	}
	h.db.ExecContext(ctx, `INSERT INTO accounts_folders (folder_id, account_id) VALUES (?, ?)`, folderID, eurAcctID)
	h.db.ExecContext(ctx, `INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, eurAcctID, seedUserID, now, now)
	// A 100 EUR expense (type=0) in Jan 2026, categorized to Food.
	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	if _, err := h.db.ExecContext(ctx, `INSERT INTO transactions (id, user_id, account_id, category_id, type, amount, description, spent_at, created_at, updated_at) VALUES (?, ?, ?, ?, 0, '100.00', '', ?, ?, ?)`,
		eurTxID, seedUserID, eurAcctID, catID, jan, now, now); err != nil {
		t.Fatalf("seed EUR tx: %v", err)
	}
}

// A USD budget with a EUR account + EUR transaction reports the EUR balance in
// its own currency, lists the EUR->USD average rate, and (in the structure)
// converts the EUR spending to the budget's USD currency.
func TestGetBudget_TwoCurrency_PerCurrencyBalancesAndRates(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.seedTwoCurrency(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Test Budget")); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}

	// Query the Jan-2026 period (past, so the period is started+ended -> non-null
	// amounts; the rate month snaps to Jan 2026).
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-01-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}

	type budgetView struct {
		Item struct {
			Balances []struct {
				CurrencyId string  `json:"currencyId"`
				Expenses   *string `json:"expenses"`
			} `json:"balances"`
			CurrencyRates []struct {
				CurrencyId     string `json:"currencyId"`
				BaseCurrencyId string `json:"baseCurrencyId"`
				Rate           string `json:"rate"`
			} `json:"currencyRates"`
			Structure struct {
				Elements []struct {
					Id    string `json:"id"`
					Spent string `json:"spent"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	res := mustUnmarshal[budgetView](t, env.Data)

	// balances: USD (budget currency, listed first) + EUR; EUR.expenses = 100.
	if len(res.Item.Balances) < 2 {
		t.Fatalf("balances=%+v want >=2 (USD + EUR)", res.Item.Balances)
	}
	if res.Item.Balances[0].CurrencyId != usdID {
		t.Fatalf("first balance currency=%q want USD (budget currency first)", res.Item.Balances[0].CurrencyId)
	}
	var eurExp *string
	for _, b := range res.Item.Balances {
		if b.CurrencyId == eurID {
			eurExp = b.Expenses
		}
	}
	if eurExp == nil || *eurExp != "100" {
		t.Fatalf("EUR expenses=%v want 100 (own currency)", eurExp)
	}

	// currencyRates: EUR->USD average = 0.91.
	var sawRate bool
	for _, r := range res.Item.CurrencyRates {
		if r.CurrencyId == eurID {
			sawRate = true
			if r.BaseCurrencyId != usdID {
				t.Errorf("rate base=%q want USD", r.BaseCurrencyId)
			}
			if r.Rate != "0.91" {
				t.Errorf("EUR rate=%q want 0.91 (avg of 0.90,0.92)", r.Rate)
			}
		}
	}
	if !sawRate {
		t.Fatalf("currencyRates=%+v want a EUR entry", res.Item.CurrencyRates)
	}

	// structure: the Food category's spent is the EUR 100 converted to USD and
	// rounded to USD's 2 fraction digits: 100 / 0.91 = 109.8901... -> 109.89.
	var foodSpent string
	for _, e := range res.Item.Structure.Elements {
		if e.Id == catID {
			foodSpent = e.Spent
		}
	}
	if foodSpent == "" {
		t.Fatalf("structure=%+v want a Food element with spent", res.Item.Structure.Elements)
	}
	if foodSpent != "109.89" {
		t.Fatalf("Food spent (converted+rounded to USD 2dp)=%q want 109.89 (100 EUR / 0.91)", foodSpent)
	}
}
