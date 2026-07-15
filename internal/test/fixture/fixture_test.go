package fixture_test

// Smoke test for the fixture builder: seed one of every entity into a real
// migrated SQLite DB and assert the rows land. This guards the builder's column
// lists + defaults against schema drift (a wrong column fails here, loudly,
// instead of in some downstream test).

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestBuilder_SeedsEveryEntity(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db).WithCrypto("0123456789abcdef")

	owner := f.User(fixture.User{Email: "owner@example.test"})
	guest := f.User(fixture.User{Email: "guest@example.test"})
	f.DefaultOptions(owner)
	f.Connect(owner, guest)

	eur := f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	f.Rate(fixture.Rate{CurrencyID: eur, Rate: "0.85000000"})
	f.Currency(fixture.Currency{Code: "PTS", Symbol: "pts", Name: "Points", UserID: owner, IsArchived: true})
	f.HiddenCurrency(guest, eur)

	folder := f.Folder(fixture.Folder{UserID: owner})
	acct := f.Account(fixture.Account{UserID: owner, Name: "Cash"})
	f.AccountInFolder(folder, acct)
	f.AccountOption(acct, owner, 0)

	shared := f.Account(fixture.Account{UserID: guest, Name: "Shared"})
	f.AccountAccess(shared, owner, 1)

	cat := f.Category(fixture.Category{UserID: owner, Name: "Food", Type: 0})
	tag := f.Tag(fixture.Tag{UserID: owner, Name: "Work"})
	payee := f.Payee(fixture.Payee{UserID: owner, Name: "Shop"})

	f.Transaction(fixture.Transaction{UserID: owner, AccountID: acct, CategoryID: cat, PayeeID: payee, TagID: tag, Type: 1, Amount: "12.50000000"})

	budget := f.Budget(fixture.Budget{UserID: owner, Name: "Trip"})
	elem := f.BudgetElement(fixture.BudgetElement{BudgetID: budget, ExternalID: cat, Type: 0, Position: 0})
	f.BudgetLimit(fixture.BudgetLimit{ElementID: elem, Period: "2024-04-01", Amount: "300.00000000"})

	// Assert representative counts.
	for _, c := range []struct {
		table string
		want  int
	}{
		{"users", 2},
		{"users_options", 4},
		{"users_connections", 2},
		{"currencies", 3}, // baseline USD + the seeded EUR + owner's custom PTS
		{"currencies_rates", 1},
		{"users_hidden_currencies", 1},
		{"folders", 1},
		{"accounts", 2},
		{"accounts_folders", 1},
		{"accounts_options", 1},
		{"accounts_access", 1},
		{"categories", 1},
		{"tags", 1},
		{"payees", 1},
		{"transactions", 1},
		{"budgets", 1},
		{"budgets_elements", 1},
		{"budgets_elements_limits", 1},
	} {
		var got int
		if err := db.Raw.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+c.table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", c.table, err)
		}
		if got != c.want {
			t.Errorf("%s: got %d rows, want %d", c.table, got, c.want)
		}
	}
}
