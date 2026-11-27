package wipetest

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/wipeguard"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestWipe_Fixture(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	fx := Seed(t, db, b)
	ctx := context.Background()
	f := fixture.New(t, db)

	pre := readDebt(t, b, fx.P, fx.D3)
	if pre.Item.CurrentBalance == fx.D3Baseline || count(t, db, `SELECT COUNT(*) FROM debts_payments WHERE debt_id = ? AND transaction_id = ?`, fx.D3, fx.D3Payment) != 1 {
		t.Fatalf("before the wipe D3 reads %s (baseline %s) and must carry U's payment split", pre.Item.CurrentBalance, fx.D3Baseline)
	}
	for _, c := range []struct{ what, query, id string }{
		{"comment", `SELECT COUNT(*) FROM budgets_elements_comments WHERE id = ?`, fx.Comment},
		{"idempotency key", `SELECT COUNT(*) FROM idempotency_keys WHERE idempotency_key = ?`, fx.IdempotencyKey},
		{"oauth state", `SELECT COUNT(*) FROM oauth_states WHERE state_hash = ?`, fx.OAuthState},
		{"grant of T on U's app", `SELECT COUNT(*) FROM oauth_grants WHERE id = ?`, fx.TGrant},
	} {
		if count(t, db, c.query, c.id) != 1 {
			t.Fatalf("the fixture's %s is missing", c.what)
		}
	}
	if n := rowsNaming(t, db, fx.U); n == 0 {
		t.Fatal("no user-naming column holds U before the wipe")
	}

	if err := b.Users.AdminMarkDeleted(ctx, fx.UEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	balancesBefore, days := balances(t, db, fx.OtherAccounts, nil)
	revsBefore := f.Revisions(fx.OtherAccounts...)
	uToP := txRow(t, db, fx.UToPTransfer)
	pToU := txRow(t, db, fx.PToUTransfer)

	out, err := b.Users.Wipe(ctx, vo.MustParseId(fx.U), nil)
	if err != nil {
		t.Fatalf("Wipe: %v", err)
	}
	if out != appuser.WipeDone {
		t.Fatalf("outcome = %q, want %q", out, appuser.WipeDone)
	}

	if count(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, fx.U) != 0 {
		t.Fatal("U's users row survived")
	}
	balancesAfter, _ := balances(t, db, fx.OtherAccounts, days)
	for acc, days := range balancesBefore {
		for d, want := range days {
			if got := balancesAfter[acc][d]; !got.Equals(want) {
				t.Errorf("account %s balance at %s = %s, want %s", acc, d, got, want)
			}
		}
	}
	f.RequireRevisionsMoved(revsBefore, fx.MovedAccounts...)
	requireSortKeys(t, db)
	if count(t, db, `SELECT COUNT(*) FROM transactions WHERE type = 2`) == 0 {
		t.Fatal("no transfer survived, so the recipient_sort_key check saw nothing")
	}

	corrections := correctionRows(t, db, fx.PCash)
	if len(corrections) != 2 {
		t.Fatalf("corrections on P cash = %+v, want 2", corrections)
	}
	for _, c := range corrections {
		switch c.typ {
		case "1":
			if !vo.NewDecimal(c.amount).Equals(vo.NewDecimal("100")) || c.user != fx.P || c.sortKey != uToP.recipientSortKey ||
				c.description != "Original transaction deleted: rent share" {
				t.Errorf("income correction = %+v, want 100 by P at %s", c, uToP.recipientSortKey)
			}
		case "0":
			if !vo.NewDecimal(c.amount).Equals(vo.NewDecimal("30")) || c.user != fx.P || c.sortKey != pToU.sortKey ||
				c.description != "Original transaction deleted" {
				t.Errorf("expense correction = %+v, want 30 by P at %s", c, pToU.sortKey)
			}
		default:
			t.Errorf("correction of type %s: %+v", c.typ, c)
		}
	}
	if n := len(correctionRows(t, db, fx.PCard)); n != 0 {
		t.Errorf("the NULL-recipient legacy transfer got %d corrections, want none", n)
	}
	for _, id := range []string{fx.UToPTransfer, fx.PToUTransfer, fx.LegacyTransfer, fx.D3Payment} {
		if count(t, db, `SELECT COUNT(*) FROM transactions WHERE id = ?`, id) != 0 {
			t.Errorf("transaction %s survived", id)
		}
	}
	if r := txRow(t, db, fx.UWroteInP); r.user != fx.P || r.category != fx.PCat1 || !vo.NewDecimal(r.amount).Equals(vo.NewDecimal("20")) ||
		r.description != "groceries by U" || r.account != fx.PCash {
		t.Errorf("U's row in P's account = %+v, want it whole and authored by P", r)
	}
	if r := txRow(t, db, fx.UWroteInX); r.user != fx.X {
		t.Errorf("U's row in X's account is authored by %s, want X", r.user)
	}
	if r := txRow(t, db, fx.PRowWithUCategory); r.category != "" {
		t.Errorf("P's row still names U's category %s", r.category)
	}

	if count(t, db, `SELECT COUNT(*) FROM budgets WHERE id = ? AND user_id = ?`, fx.TBudget, fx.T) != 1 ||
		count(t, db, `SELECT COUNT(*) FROM budgets_access WHERE budget_id = ? AND user_id = ?`, fx.TBudget, fx.P) != 1 {
		t.Error("P and T no longer share T's budget")
	}
	if count(t, db, `SELECT COUNT(*) FROM users_options WHERE user_id = ? AND name = 'budget' AND value = ?`, fx.P, fx.UBudget) != 0 {
		t.Error("P's active budget still points at U's deleted budget")
	}
	if count(t, db, `SELECT COUNT(*) FROM debts WHERE id = ?`, fx.D1) != 0 {
		t.Error("debt D1 on U's category survived")
	}
	if count(t, db, `SELECT COUNT(*) FROM debts_payments WHERE debt_id = ?`, fx.D3) != 0 {
		t.Error("D3 still has a payment split")
	}
	post := readDebt(t, b, fx.P, fx.D3)
	if post.Item.CurrentBalance != fx.D3Baseline || len(post.Payments) != 0 {
		t.Errorf("D3 after the wipe reads %s with %d payments, want its snapshot's %s and none", post.Item.CurrentBalance, len(post.Payments), fx.D3Baseline)
	}
	if count(t, db, `SELECT COUNT(*) FROM budgets WHERE id = ? AND currency_id = ?`, fx.PBudget, fx.UCurrency) != 1 ||
		count(t, db, `SELECT COUNT(*) FROM debts WHERE id = ? AND currency_id = ?`, fx.D2, fx.UCurrency) != 1 ||
		count(t, db, `SELECT COUNT(*) FROM currencies WHERE id = ? AND user_id = ?`, fx.UCurrency, fx.P) != 1 {
		t.Error("P's budget or debt D2 lost its currency, or the currency did not move to P")
	}
	readDebt(t, b, fx.P, fx.D2)
	if count(t, db, `SELECT COUNT(*) FROM oauth_grants WHERE id = ?`, fx.TGrant) != 0 {
		t.Error("T's grant on U's app survived")
	}

	for _, c := range wipeguard.UserColumns(t, db) {
		if c.Table == "users_audit" {
			continue
		}
		if n := count(t, db, `SELECT COUNT(*) FROM `+c.Table+` WHERE `+c.Name+` = ?`, fx.U); n != 0 {
			t.Errorf("%s holds U's id in %d rows", c, n)
		}
	}
	requireDeletedAudit(t, db, fx.U, `{"reason":"cli","lifetimeAccess":true}`, model.EmailHash(fx.UEmail))
}

func TestWipe_EmptyUser(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	u := fixture.New(t, db).User(fixture.User{Email: "empty@wipe.test"})
	markAndWipe(t, b, u, "empty@wipe.test", appuser.WipeDone)
	if count(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, u) != 0 {
		t.Fatal("the users row survived")
	}
}

func TestWipe_HashIsOfTheCurrentEmail(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	u := fixture.New(t, db).User(fixture.User{Email: "old@wipe.test"})
	if err := b.Users.AdminMarkDeleted(context.Background(), "old@wipe.test"); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	db.Exec(t, db.Rebind(`UPDATE users SET email = ? WHERE id = ?`), "new@wipe.test", u)
	if out, err := b.Users.Wipe(context.Background(), vo.MustParseId(u), nil); err != nil || out != appuser.WipeDone {
		t.Fatalf("Wipe = %q, %v", out, err)
	}
	requireDeletedAudit(t, db, u, `{"reason":"cli","lifetimeAccess":true}`, model.EmailHash("new@wipe.test"))
}

func TestWipe_NotLifetime(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	until := Today.Add(30 * 24 * time.Hour)
	u := fixture.New(t, db).User(fixture.User{Email: "trial@wipe.test", AccessUntil: &until})
	markAndWipe(t, b, u, "trial@wipe.test", appuser.WipeDone)
	requireDeletedAudit(t, db, u, `{"reason":"cli","lifetimeAccess":false}`, "")
	if count(t, db, `SELECT COUNT(*) FROM users_audit WHERE user_id = ? AND event = ? AND email_hash IS NULL`, u, model.AuditDeleted) != 1 {
		t.Fatal("email_hash is not NULL for an account without lifetime access")
	}
}

func TestWipe_NotMarked(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	u := fixture.New(t, db).User(fixture.User{Email: "kept@wipe.test"})
	out, err := b.Users.Wipe(context.Background(), vo.MustParseId(u), nil)
	if err != nil || out != appuser.WipeNotMarked {
		t.Fatalf("Wipe = %q, %v; want %q", out, err, appuser.WipeNotMarked)
	}
	if count(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, u) != 1 {
		t.Fatal("an unmarked user was deleted")
	}
	if out, err := b.Users.Wipe(context.Background(), vo.MustParseId(fixture.NewID()), nil); err != nil || out != appuser.WipeNotMarked {
		t.Fatalf("Wipe of a missing user = %q, %v; want %q", out, err, appuser.WipeNotMarked)
	}
}

func TestWipe_SelectedMarkMismatch(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	ctx := context.Background()
	u := fixture.New(t, db).User(fixture.User{Email: "remarked@wipe.test"})
	if err := b.Users.AdminMarkDeleted(ctx, "remarked@wipe.test"); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	st, err := userrepo.NewDeletionRepo(db.Engine, db.TX).GetState(ctx, vo.MustParseId(u))
	if err != nil || st.MarkedAt == nil {
		t.Fatalf("GetState = %+v, %v", st, err)
	}
	other := st.MarkedAt.Add(-24 * time.Hour)
	out, err := b.Users.Wipe(ctx, vo.MustParseId(u), &other)
	if err != nil || out != appuser.WipeNotMarked {
		t.Fatalf("Wipe with another mark = %q, %v; want %q", out, err, appuser.WipeNotMarked)
	}
	if count(t, db, `SELECT COUNT(*) FROM users WHERE id = ? AND deletion_marked_at IS NOT NULL`, u) != 1 {
		t.Fatal("a mismatched wipe changed the user")
	}
	selected := *st.MarkedAt
	if out, err := b.Users.Wipe(ctx, vo.MustParseId(u), &selected); err != nil || out != appuser.WipeDone {
		t.Fatalf("Wipe with the selected mark = %q, %v; want %q", out, err, appuser.WipeDone)
	}
}

var errHalfway = errors.New("wipe failed halfway")

type failingWiper struct{ real *server.UserDataWiper }

func (w failingWiper) LockBudgets(ctx context.Context, userID vo.Id) error {
	return w.real.LockBudgets(ctx, userID)
}

func (w failingWiper) Erase(ctx context.Context, userID vo.Id) error {
	if err := w.real.Erase(ctx, userID); err != nil {
		return err
	}
	return errHalfway
}

func TestWipe_RollsBackWhole(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	fx := Seed(t, db, b)
	ctx := context.Background()
	if err := b.Users.AdminMarkDeleted(ctx, fx.UEmail); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	snapshot := func() []string {
		var out []string
		for _, q := range []struct {
			query string
			args  []any
		}{
			{`SELECT id, email, deletion_marked_at FROM users WHERE id = ?`, []any{fx.U}},
			{`SELECT account_id, user_id, role, is_accepted FROM accounts_access WHERE user_id = ? OR account_id IN (SELECT id FROM accounts WHERE user_id = ?) ORDER BY account_id, user_id`, []any{fx.U, fx.U}},
			{`SELECT budget_id, user_id, role, is_accepted FROM budgets_access WHERE user_id = ? OR budget_id IN (SELECT id FROM budgets WHERE user_id = ?) ORDER BY budget_id, user_id`, []any{fx.U, fx.U}},
			{`SELECT id, user_id, type, account_id, account_recipient_id, amount, amount_recipient, sort_key, recipient_sort_key FROM transactions
				WHERE account_id IN (?, ?) OR account_recipient_id IN (?, ?) ORDER BY id`, []any{fx.PCash, fx.PCard, fx.PCash, fx.PCard}},
		} {
			out = append(out, rowStrings(t, db, q.query, q.args...)...)
			out = append(out, "--")
		}
		return out
	}
	before := snapshot()
	b.Users.SetDataWiper(failingWiper{real: b.Wiper})
	if _, err := b.Users.Wipe(ctx, vo.MustParseId(fx.U), nil); !errors.Is(err, errHalfway) {
		t.Fatalf("Wipe error = %v, want %v", err, errHalfway)
	}
	after := snapshot()
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("a failed wipe left changes:\nbefore:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
}

func TestWipe_Unwired(t *testing.T) {
	db := dbtest.New(t)
	b := Build(t, db)
	u := fixture.New(t, db).User(fixture.User{Email: "unwired@wipe.test"})
	if err := b.Users.AdminMarkDeleted(context.Background(), "unwired@wipe.test"); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	b.Users.SetDataWiper(nil)
	if _, err := b.Users.Wipe(context.Background(), vo.MustParseId(u), nil); err == nil {
		t.Fatal("a wipe without a wiper succeeded")
	}
	if count(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, u) != 1 {
		t.Fatal("an unwired wipe deleted the user")
	}
}

func markAndWipe(t *testing.T, b *server.Built, userID, email string, want appuser.WipeOutcome) {
	t.Helper()
	ctx := context.Background()
	if err := b.Users.AdminMarkDeleted(ctx, email); err != nil {
		t.Fatalf("AdminMarkDeleted: %v", err)
	}
	out, err := b.Users.Wipe(ctx, vo.MustParseId(userID), nil)
	if err != nil || out != want {
		t.Fatalf("Wipe = %q, %v; want %q", out, err, want)
	}
}

func requireDeletedAudit(t *testing.T, db *dbtest.DB, userID, details, emailHash string) {
	t.Helper()
	rows := rowStrings(t, db, `SELECT details, email_hash FROM users_audit WHERE user_id = ? AND event = ?`, userID, model.AuditDeleted)
	want := details + "|" + emailHash
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("deleted audit rows = %q, want exactly [%q]", rows, want)
	}
}

func count(t *testing.T, db *dbtest.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Raw.QueryRowContext(context.Background(), db.Rebind(query), args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// rowStrings renders each row as its columns joined by "|", NULL as "".
func rowStrings(t *testing.T, db *dbtest.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.Raw.QueryContext(context.Background(), db.Rebind(query), args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = v.String
		}
		out = append(out, strings.Join(parts, "|"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func rowsNaming(t *testing.T, db *dbtest.DB, userID string) int {
	t.Helper()
	total := 0
	for _, c := range wipeguard.UserColumns(t, db) {
		if c.Table != "users_audit" {
			total += count(t, db, `SELECT COUNT(*) FROM `+c.Table+` WHERE `+c.Name+` = ?`, userID)
		}
	}
	return total
}

type txFields struct {
	user, account, category, typ, amount, description, sortKey, recipientSortKey string
}

func txRow(t *testing.T, db *dbtest.DB, id string) txFields {
	t.Helper()
	rows := rowStrings(t, db, `SELECT user_id, account_id, category_id, type, amount, description, sort_key, recipient_sort_key FROM transactions WHERE id = ?`, id)
	if len(rows) != 1 {
		t.Fatalf("transaction %s: %d rows", id, len(rows))
	}
	return splitTx(rows[0])
}

func splitTx(row string) txFields {
	p := strings.Split(row, "|")
	return txFields{user: p[0], account: p[1], category: p[2], typ: p[3], amount: p[4], description: p[5], sortKey: p[6], recipientSortKey: p[7]}
}

func correctionRows(t *testing.T, db *dbtest.DB, accountID string) []txFields {
	t.Helper()
	var out []txFields
	for _, r := range rowStrings(t, db, `SELECT user_id, account_id, category_id, type, amount, description, sort_key, recipient_sort_key FROM transactions
		WHERE account_id = ? AND description LIKE 'Original transaction deleted%' ORDER BY id`, accountID) {
		out = append(out, splitTx(r))
	}
	return out
}

// balances are each account's balance at the end of every day in dayList, or,
// when it is nil, of every day a row of these accounts falls on, plus "all";
// a transfer counts amount_recipient on its recipient side, as the production
// balance query does.
func balances(t *testing.T, db *dbtest.DB, accounts, dayList []string) (map[string]map[string]vo.DecimalNumber, []string) {
	t.Helper()
	type move struct {
		day     string
		account string
		delta   vo.DecimalNumber
	}
	in := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		in[a] = true
	}
	var moves []move
	days := map[string]bool{}
	for _, r := range rowStrings(t, db, `SELECT type, account_id, account_recipient_id, amount, amount_recipient, spent_at FROM transactions`) {
		p := strings.Split(r, "|")
		typ, src, dst, amount, amountRecipient, d := p[0], p[1], p[2], vo.NewDecimal(p[3]), p[4], p[5][:10]
		zero := vo.NewDecimal("0")
		if in[src] {
			days[d] = true
			if typ == "1" {
				moves = append(moves, move{d, src, amount})
			} else {
				moves = append(moves, move{d, src, zero.Sub(amount)})
			}
		}
		if typ == "2" && in[dst] {
			days[d] = true
			if amountRecipient != "" {
				moves = append(moves, move{d, dst, vo.NewDecimal(amountRecipient)})
			}
		}
	}
	if dayList == nil {
		for d := range days {
			dayList = append(dayList, d)
		}
		sort.Strings(dayList)
		dayList = append(dayList, "all")
	}
	out := make(map[string]map[string]vo.DecimalNumber, len(accounts))
	for _, a := range accounts {
		out[a] = map[string]vo.DecimalNumber{}
		for _, d := range dayList {
			sum := vo.NewDecimal("0")
			for _, m := range moves {
				if m.account == a && (d == "all" || m.day <= d) {
					sum = sum.Add(m.delta)
				}
			}
			out[a][d] = sum
		}
	}
	return out, dayList
}

// requireSortKeys carries revisionguard's two storage invariants, which the
// wipe (having no route) is not covered by: every row has its keys, and no
// two rows of one account's day share a key.
func requireSortKeys(t *testing.T, db *dbtest.DB) {
	t.Helper()
	seen := map[string]string{}
	for _, r := range rowStrings(t, db, `SELECT id, type, account_id, account_recipient_id, sort_key, recipient_sort_key, spent_at FROM transactions`) {
		p := strings.Split(r, "|")
		id, typ, src, dst, key, rkey, d := p[0], p[1], p[2], p[3], p[4], p[5], p[6][:10]
		if key == "" {
			t.Errorf("transaction %s has no sort_key", id)
		}
		if typ == "2" && rkey == "" {
			t.Errorf("transfer %s has no recipient_sort_key", id)
		}
		check := func(account, k string) {
			if k == "" {
				return
			}
			slot := account + "|" + d + "|" + k
			if other, dup := seen[slot]; dup {
				t.Errorf("transactions %s and %s share key %s on account %s, %s", other, id, k, account, d)
			}
			seen[slot] = id
		}
		check(src, key)
		if typ == "2" {
			check(dst, rkey)
		}
	}
}
