package migrations_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	ctU1       = "01a00000-0000-7000-8000-000000000001"
	ctA1       = "01a00000-0000-7000-8000-0000000000a1"
	ctA2       = "01a00000-0000-7000-8000-0000000000a2"
	ctA3       = "01a00000-0000-7000-8000-0000000000a3"
	ctFood     = "01a00000-0000-7000-8000-0000000000c1"
	ctSalary   = "01a00000-0000-7000-8000-0000000000c2"
	ctRefund   = "01a00000-0000-7000-8000-0000000000d1"
	ctWrong    = "01a00000-0000-7000-8000-0000000000d2"
	ctLong     = "01a00000-0000-7000-8000-0000000000d3"
	ctFine     = "01a00000-0000-7000-8000-0000000000d4"
	ctTransfer = "01a00000-0000-7000-8000-0000000000d5"
	ctRt       = "01a00000-0000-7000-8000-0000000000e1"
)

// 20261017000000 clears a category of the other type (an expense category on
// income or the reverse) from transactions and recurring templates and notes
// the removal in English, keeping the note whole within 255 characters.
func TestMigration20261017_RemovesCategoriesOfTheOtherType(t *testing.T) {
	db := runUpTo(t, "category_type", "20261017000000")
	checkCategoryTypeMigration(t, db, func(q string) string { return q }, func() { runThrough(t, db, "20261017000000") })
}

// checkCategoryTypeMigration seeds a database migrated up to just before
// 20261017000000, runs the rest (finish) and checks the outcome; shared by the
// SQLite test above and the PostgreSQL one (enginecompare tag). rebind turns
// the ? placeholders into the engine's own.
func checkCategoryTypeMigration(t *testing.T, db *sql.DB, rebind func(string) string, finish func()) {
	t.Helper()
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, rebind(q), args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, 'u1@x.test', 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, ctU1, ctU1)
	for _, a := range []string{ctA1, ctA2, ctA3} {
		mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'A', 2, 'wallet', FALSE, `+ts+`, `+ts+`)`, a, usdSeed, ctU1)
	}
	mustExec(`INSERT INTO categories (id, user_id, name, type, icon, created_at, updated_at) VALUES (?, ?, 'Еда', 0, 'i', `+ts+`, `+ts+`)`, ctFood, ctU1)
	mustExec(`INSERT INTO categories (id, user_id, name, type, icon, created_at, updated_at) VALUES (?, ?, 'Salary', 1, 'i', `+ts+`, `+ts+`)`, ctSalary, ctU1)
	tx := func(id, account string, typ int, cat any, desc string, recipient any) {
		mustExec(`INSERT INTO transactions (id, user_id, account_id, account_recipient_id, category_id, description, type, amount, spent_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 5, `+ts+`, `+ts+`, `+ts+`)`, id, ctU1, account, recipient, cat, desc, typ)
	}
	long := strings.Repeat("я", 250)
	tx(ctRefund, ctA1, 1, ctFood, "refund  \n", nil)
	tx(ctWrong, ctA1, 0, ctSalary, "", nil)
	tx(ctLong, ctA1, 1, ctFood, long, nil)
	tx(ctFine, ctA2, 0, ctFood, "lunch", nil)
	tx(ctTransfer, ctA2, 2, nil, "", ctA3)
	mustExec(`INSERT INTO recurring_transactions (id, user_id, account_id, category_id, type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at) VALUES (?, ?, ?, ?, 1, 5, 'rent', 'monthly', `+ts+`, 1, `+ts+`, `+ts+`)`, ctRt, ctU1, ctA2, ctFood)

	finish()

	row := func(table, id string) (cat sql.NullString, desc, updated string) {
		t.Helper()
		if err := db.QueryRowContext(ctx, rebind(`SELECT category_id, description, CAST(updated_at AS TEXT) FROM `+table+` WHERE id = ?`), id).Scan(&cat, &desc, &updated); err != nil {
			t.Fatal(err)
		}
		return
	}
	const expenseNote = "Category “Еда” removed: an expense category can't classify income."
	if cat, desc, updated := row("transactions", ctRefund); cat.Valid || desc != "refund\n"+expenseNote || updated == "2026-01-01 00:00:00" || len(updated) != 19 {
		t.Errorf("income on an expense category: %v %q %q", cat, desc, updated)
	}
	if cat, desc, _ := row("transactions", ctWrong); cat.Valid || desc != "Category “Salary” removed: an income category can't classify an expense." {
		t.Errorf("expense on an income category: %v %q", cat, desc)
	}
	if cat, desc, _ := row("transactions", ctLong); cat.Valid || utf8.RuneCountInString(desc) != 255 || !strings.HasSuffix(desc, "…\n"+expenseNote) || !strings.HasPrefix(desc, "яяя") {
		t.Errorf("a long description is shortened, the note kept whole: %d %q", utf8.RuneCountInString(desc), desc)
	}
	if cat, desc, updated := row("transactions", ctFine); cat.String != ctFood || desc != "lunch" || updated != "2026-01-01 00:00:00" {
		t.Errorf("a matching row is untouched: %v %q %q", cat, desc, updated)
	}
	if cat, desc, _ := row("recurring_transactions", ctRt); cat.Valid || desc != "rent\n"+expenseNote {
		t.Errorf("template: %v %q", cat, desc)
	}
	revision := func(id string) (r int) {
		if err := db.QueryRowContext(ctx, rebind(`SELECT transactions_revision FROM accounts WHERE id = ?`), id).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return
	}
	if revision(ctA1) != 1 || revision(ctA2) != 0 || revision(ctA3) != 0 {
		t.Errorf("only the account holding cleared rows moves: a1=%d a2=%d a3=%d", revision(ctA1), revision(ctA2), revision(ctA3))
	}
}
