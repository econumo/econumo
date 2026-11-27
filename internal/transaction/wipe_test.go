package transaction_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
)

const wipeDay = "2026-03-10 12:00:00"

type wipeEnv struct {
	t        *testing.T
	db       *dbtest.DB
	f        *fixture.Builder
	svc      *apptransaction.Service
	accounts *accountrepo.Repo
	u, p     string
}

func newWipeEnv(t *testing.T) *wipeEnv {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	svc := apptransaction.NewService(transactionrepo.NewRepo(db.Engine, db.TX), nil, nil, nil, nil, nil, nil, nil, nil, db.TX, nil, clock.New())
	return &wipeEnv{
		t: t, db: db, f: f, svc: svc,
		accounts: accountrepo.NewRepo(db.Engine, db.TX),
		u:        f.User(fixture.User{Name: "Wiped", Email: "wipe-tx-u@example.test"}),
		p:        f.User(fixture.User{Name: "Partner", Email: "wipe-tx-p@example.test"}),
	}
}

func (e *wipeEnv) rewrite() {
	e.t.Helper()
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		return e.svc.RewriteForDeletedUser(ctx, vo.MustParseId(e.u))
	})
	if err != nil {
		e.t.Fatalf("RewriteForDeletedUser: %v", err)
	}
}

var wipeCutoffs = []string{
	"2026-03-09 00:00:00", "2026-03-10 00:00:00", "2026-03-10 12:00:00",
	"2026-03-10 12:00:01", "2026-03-11 00:00:00", "2026-04-01 00:00:00",
}

func (e *wipeEnv) balances(accountID string) []string {
	e.t.Helper()
	out := make([]string, len(wipeCutoffs))
	for i, c := range wipeCutoffs {
		at, err := time.Parse("2006-01-02 15:04:05", c)
		if err != nil {
			e.t.Fatal(err)
		}
		b, err := e.accounts.Balance(context.Background(), vo.MustParseId(accountID), at)
		if err != nil {
			e.t.Fatalf("balance %s at %s: %v", accountID, c, err)
		}
		out[i] = c + "=" + b
	}
	return out
}

func (e *wipeEnv) requireSameBalances(accountID string, before []string) {
	e.t.Helper()
	after := e.balances(accountID)
	for i := range before {
		if before[i] != after[i] {
			e.t.Errorf("account %s balance changed: before %s, after %s", accountID, before[i], after[i])
		}
	}
}

// neighbours puts rows keyed below and above "m" on the account's transfer
// day, plus one on the previous day, so a misplaced or missing correction
// shows in the balances and in the key check.
func (e *wipeEnv) neighbours(owner, accountID string) {
	e.f.Transaction(fixture.Transaction{UserID: owner, AccountID: accountID, Type: 0, Amount: "3", SpentAt: wipeDay, SortKey: "a"})
	e.f.Transaction(fixture.Transaction{UserID: owner, AccountID: accountID, Type: 1, Amount: "100", SpentAt: wipeDay, SortKey: "z"})
	e.f.Transaction(fixture.Transaction{UserID: owner, AccountID: accountID, Type: 1, Amount: "50", SpentAt: "2026-03-09 08:00:00"})
}

func (e *wipeEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.Raw.QueryRow(e.db.Rebind(query), args...).Scan(&n); err != nil {
		e.t.Fatalf("count %q: %v", query, err)
	}
	return n
}

type wipeRow struct {
	ID, UserID, Description, SortKey                             string
	Type                                                         int
	Amount                                                       string
	SpentAt                                                      time.Time
	Recipient, Category, Payee, Tag, AmountRecipient, RecipientK *string
}

func (e *wipeEnv) correctionsOn(accountID string) []wipeRow {
	e.t.Helper()
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT id, user_id, description, sort_key, type, amount, spent_at,
		account_recipient_id, category_id, payee_id, tag_id, amount_recipient, recipient_sort_key
		FROM transactions WHERE account_id = ? AND description LIKE 'Original transaction deleted%' ORDER BY sort_key`), accountID)
	if err != nil {
		e.t.Fatalf("list corrections: %v", err)
	}
	defer rows.Close()
	var out []wipeRow
	for rows.Next() {
		var r wipeRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Description, &r.SortKey, &r.Type, &r.Amount, &r.SpentAt,
			&r.Recipient, &r.Category, &r.Payee, &r.Tag, &r.AmountRecipient, &r.RecipientK); err != nil {
			e.t.Fatalf("scan correction: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

// snapshot renders every column of a transaction row (and its labels), so a
// before/after comparison catches any change.
func (e *wipeEnv) snapshot(id string) string {
	e.t.Helper()
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT * FROM transactions WHERE id = ?`), id)
	if err != nil {
		e.t.Fatalf("snapshot %s: %v", id, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		e.t.Fatal(err)
	}
	if !rows.Next() {
		return "<missing>"
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		e.t.Fatal(err)
	}
	rows.Close()
	var sb strings.Builder
	for i, c := range cols {
		v := vals[i]
		if b, ok := v.([]byte); ok {
			v = string(b)
		}
		fmt.Fprintf(&sb, "%s=%v;", c, v)
	}
	fmt.Fprintf(&sb, "labels=%d", e.count(`SELECT COUNT(*) FROM transactions_labels WHERE transaction_id = ?`, id))
	return sb.String()
}

func (e *wipeEnv) column(id, col string) string {
	e.t.Helper()
	var v string
	if err := e.db.Raw.QueryRow(e.db.Rebind(`SELECT `+col+` FROM transactions WHERE id = ?`), id).Scan(&v); err != nil {
		e.t.Fatalf("read %s of %s: %v", col, id, err)
	}
	return v
}

// requireUniqueKeys checks that no two rows of one account's day share a key
// on that account's side.
func (e *wipeEnv) requireUniqueKeys(accountIDs ...string) {
	e.t.Helper()
	for _, acct := range accountIDs {
		rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT spent_at, sort_key FROM transactions WHERE account_id = ?
			UNION ALL SELECT spent_at, recipient_sort_key FROM transactions WHERE account_recipient_id = ?`), acct, acct)
		if err != nil {
			e.t.Fatalf("keys of %s: %v", acct, err)
		}
		seen := map[string]bool{}
		for rows.Next() {
			var at time.Time
			var key *string
			if err := rows.Scan(&at, &key); err != nil {
				e.t.Fatal(err)
			}
			if key == nil || *key == "" {
				e.t.Errorf("account %s: row on %s has no key on its side", acct, at)
				continue
			}
			k := at.Format("2006-01-02") + "/" + *key
			if seen[k] {
				e.t.Errorf("account %s: key %s repeated", acct, k)
			}
			seen[k] = true
		}
		if err := rows.Err(); err != nil {
			e.t.Fatal(err)
		}
		rows.Close()
	}
}

func decimalEq(t *testing.T, got, want string) {
	t.Helper()
	if vo.NewDecimal(got).String() != vo.NewDecimal(want).String() {
		t.Errorf("amount = %s, want %s", got, want)
	}
}

func TestRewrite_ReassignsForeignAuthoredRows(t *testing.T) {
	e := newWipeEnv(t)
	tu := e.f.User(fixture.User{Name: "Third", Email: "wipe-tx-t@example.test"})
	x := e.f.User(fixture.User{Name: "Ex", Email: "wipe-tx-x@example.test"})
	pAcc := e.f.Account(fixture.Account{UserID: e.p})
	pCtl := e.f.Account(fixture.Account{UserID: e.p})
	tAcc := e.f.Account(fixture.Account{UserID: tu})
	xAcc := e.f.Account(fixture.Account{UserID: x})
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	cat := e.f.Category(fixture.Category{UserID: e.p})
	payee := e.f.Payee(fixture.Payee{UserID: e.p})
	tag := e.f.Tag(fixture.Tag{UserID: e.p})
	label := e.f.Label(fixture.Label{UserID: e.p})

	expense := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: pAcc, Type: 0, Amount: "7", Description: "lunch",
		CategoryID: cat, PayeeID: payee, TagID: tag, SpentAt: wipeDay, SortKey: "b"})
	if _, err := e.db.Raw.Exec(e.db.Rebind(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`), expense, label); err != nil {
		t.Fatal(err)
	}
	income := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: pAcc, Type: 1, Amount: "8", SpentAt: wipeDay, SortKey: "c"})
	transfer := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: pAcc, AccountRecipientID: tAcc, Type: 2, Amount: "5", AmountRecipient: "5",
		SpentAt: wipeDay, SortKey: "d", RecipientSortKey: "d"})
	exRow := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: xAcc, Type: 0, Amount: "2", SpentAt: wipeDay})
	e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: pCtl, Type: 0, Amount: "1", SpentAt: wipeDay})
	own := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: uAcc, Type: 0, Amount: "1", SpentAt: wipeDay})

	want := map[string]string{expense: e.p, income: e.p, transfer: e.p, exRow: x, own: e.u}
	before := map[string]string{}
	for id := range want {
		before[id] = e.snapshot(id)
	}
	revs := e.f.Revisions(pAcc, pCtl, tAcc, xAcc, uAcc)

	e.rewrite()

	for id, owner := range want {
		if got := e.column(id, "user_id"); got != owner {
			t.Errorf("row %s user_id = %s, want %s", id, got, owner)
		}
		wantSnap := strings.Replace(before[id], "user_id="+e.u+";", "user_id="+owner+";", 1)
		if got := e.snapshot(id); got != wantSnap {
			t.Errorf("row %s changed beyond its author:\nbefore %s\nafter  %s", id, wantSnap, got)
		}
	}
	e.f.RequireRevisionsMoved(revs, pAcc, tAcc, xAcc)
	e.requireUniqueKeys(pAcc, pCtl, tAcc, xAcc, uAcc)
}

func TestRewrite_TransferFromPartnerToUser(t *testing.T) {
	e := newWipeEnv(t)
	pAcc := e.f.Account(fixture.Account{UserID: e.p})
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	e.neighbours(e.p, pAcc)
	orig := e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: pAcc, AccountRecipientID: uAcc, Type: 2, Amount: "10", AmountRecipient: "10",
		Description: "rent", SpentAt: wipeDay, SortKey: "m", RecipientSortKey: "k"})
	bal := e.balances(pAcc)
	revs := e.f.Revisions(pAcc)

	e.rewrite()

	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE id = ?`, orig); n != 0 {
		t.Fatalf("original transfer still present")
	}
	cs := e.correctionsOn(pAcc)
	if len(cs) != 1 {
		t.Fatalf("corrections on P.acc = %d, want 1", len(cs))
	}
	c := cs[0]
	if c.Type != 0 || c.UserID != e.p || c.Description != "Original transaction deleted: rent" || c.SortKey != "m" {
		t.Errorf("correction = %+v", c)
	}
	decimalEq(t, c.Amount, "10")
	if !c.SpentAt.Equal(mustTime(t, wipeDay)) {
		t.Errorf("spent_at = %s", c.SpentAt)
	}
	if c.Recipient != nil || c.Category != nil || c.Payee != nil || c.Tag != nil || c.AmountRecipient != nil || c.RecipientK != nil {
		t.Errorf("correction carries references: %+v", c)
	}
	e.requireSameBalances(pAcc, bal)
	e.f.RequireRevisionsMoved(revs, pAcc)
	e.requireUniqueKeys(pAcc)
}

func TestRewrite_TransferFromUserToPartner(t *testing.T) {
	e := newWipeEnv(t)
	pAcc := e.f.Account(fixture.Account{UserID: e.p})
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	e.neighbours(e.p, pAcc)
	orig := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: uAcc, AccountRecipientID: pAcc, Type: 2, Amount: "10", AmountRecipient: "9.5",
		SpentAt: wipeDay, SortKey: "q", RecipientSortKey: "m"})
	bal := e.balances(pAcc)
	revs := e.f.Revisions(pAcc)

	e.rewrite()

	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE id = ?`, orig); n != 0 {
		t.Fatalf("original transfer still present")
	}
	cs := e.correctionsOn(pAcc)
	if len(cs) != 1 {
		t.Fatalf("corrections on P.acc = %d, want 1", len(cs))
	}
	c := cs[0]
	if c.Type != 1 || c.UserID != e.p || c.SortKey != "m" || c.Description != "Original transaction deleted" || c.Recipient != nil {
		t.Errorf("correction = %+v", c)
	}
	decimalEq(t, c.Amount, "9.5")
	e.requireSameBalances(pAcc, bal)
	e.f.RequireRevisionsMoved(revs, pAcc)
	e.requireUniqueKeys(pAcc)
}

func TestRewrite_NullRecipientAmount(t *testing.T) {
	e := newWipeEnv(t)
	pAcc := e.f.Account(fixture.Account{UserID: e.p})
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	e.neighbours(e.p, pAcc)
	orig := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: uAcc, AccountRecipientID: pAcc, Type: 2, Amount: "10",
		SpentAt: wipeDay, SortKey: "q", RecipientSortKey: "m"})
	bal := e.balances(pAcc)
	revs := e.f.Revisions(pAcc)
	rowsBefore := e.count(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`, pAcc)

	e.rewrite()

	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE id = ?`, orig); n != 0 {
		t.Fatalf("original transfer still present")
	}
	if n := e.count(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`, pAcc); n != rowsBefore {
		t.Errorf("rows on P.acc = %d, want %d (no correction)", n, rowsBefore)
	}
	e.requireSameBalances(pAcc, bal)
	e.f.RequireRevisionsMoved(revs, pAcc)
	e.requireUniqueKeys(pAcc)
}

func TestRewrite_EmptyAndLongDescriptions(t *testing.T) {
	e := newWipeEnv(t)
	pEmpty := e.f.Account(fixture.Account{UserID: e.p})
	pLong := e.f.Account(fixture.Account{UserID: e.p})
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	long := strings.Repeat("ж", 255)
	e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: pEmpty, AccountRecipientID: uAcc, Type: 2, Amount: "1", AmountRecipient: "1", SpentAt: wipeDay})
	e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: pLong, AccountRecipientID: uAcc, Type: 2, Amount: "1", AmountRecipient: "1", Description: long, SpentAt: wipeDay})

	e.rewrite()

	if cs := e.correctionsOn(pEmpty); len(cs) != 1 || cs[0].Description != "Original transaction deleted" {
		t.Errorf("empty description corrections = %+v", cs)
	}
	cs := e.correctionsOn(pLong)
	if len(cs) != 1 {
		t.Fatalf("long description corrections = %d, want 1", len(cs))
	}
	d := cs[0].Description
	if n := utf8.RuneCountInString(d); n != 255 {
		t.Errorf("description runes = %d, want 255", n)
	}
	if !strings.HasPrefix(d, "Original transaction deleted: жж") {
		t.Errorf("description = %q", d)
	}
	e.requireUniqueKeys(pEmpty, pLong)
}

func TestRewrite_SoftDeletedAccounts(t *testing.T) {
	for _, deleted := range []string{"partner", "user"} {
		t.Run(deleted, func(t *testing.T) {
			e := newWipeEnv(t)
			pOut := e.f.Account(fixture.Account{UserID: e.p, Deleted: deleted == "partner"})
			pIn := e.f.Account(fixture.Account{UserID: e.p, Deleted: deleted == "partner"})
			uAcc := e.f.Account(fixture.Account{UserID: e.u, Deleted: deleted == "user"})
			e.neighbours(e.p, pOut)
			e.neighbours(e.p, pIn)
			e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: pOut, AccountRecipientID: uAcc, Type: 2, Amount: "10", AmountRecipient: "10",
				SpentAt: wipeDay, SortKey: "m", RecipientSortKey: "m"})
			e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: uAcc, AccountRecipientID: pIn, Type: 2, Amount: "10", AmountRecipient: "9.5",
				SpentAt: wipeDay, SortKey: "n", RecipientSortKey: "m"})
			balOut, balIn := e.balances(pOut), e.balances(pIn)

			e.rewrite()

			if cs := e.correctionsOn(pOut); len(cs) != 1 || cs[0].Type != 0 || cs[0].SortKey != "m" {
				t.Errorf("outgoing corrections = %+v", cs)
			}
			if cs := e.correctionsOn(pIn); len(cs) != 1 || cs[0].Type != 1 || cs[0].SortKey != "m" {
				t.Errorf("incoming corrections = %+v", cs)
			}
			e.requireSameBalances(pOut, balOut)
			e.requireSameBalances(pIn, balIn)
			e.requireUniqueKeys(pOut, pIn)
		})
	}
}

func TestRewrite_BumpsAccountsThatLoseReferences(t *testing.T) {
	e := newWipeEnv(t)
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	acc := func() string { return e.f.Account(fixture.Account{UserID: e.p}) }
	accCat, accPayee, accTag, accLabel, accImport, accRecU, accRecP, accCtl := acc(), acc(), acc(), acc(), acc(), acc(), acc(), acc()

	row := func(account string, tx fixture.Transaction) string {
		tx.UserID, tx.AccountID, tx.Type, tx.Amount, tx.SpentAt = e.p, account, 0, "1", wipeDay
		return e.f.Transaction(tx)
	}
	var ids []string
	ids = append(ids, row(accCat, fixture.Transaction{CategoryID: e.f.Category(fixture.Category{UserID: e.u})}))
	ids = append(ids, row(accPayee, fixture.Transaction{PayeeID: e.f.Payee(fixture.Payee{UserID: e.u})}))
	ids = append(ids, row(accTag, fixture.Transaction{TagID: e.f.Tag(fixture.Tag{UserID: e.u})}))
	labelled := row(accLabel, fixture.Transaction{})
	ids = append(ids, labelled)
	if _, err := e.db.Raw.Exec(e.db.Rebind(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`),
		labelled, e.f.Label(fixture.Label{UserID: e.u})); err != nil {
		t.Fatal(err)
	}
	imported := row(accImport, fixture.Transaction{})
	ids = append(ids, imported)
	e.f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: e.f.ImportSource(fixture.ImportSource{UserID: e.u}),
		ExternalAccountID: "ext", ExternalTransactionID: "ext-1", TransactionID: imported})

	template := func(owner, account, recipient string) string {
		id := vo.NewId().String()
		var rcpt any
		if recipient != "" {
			rcpt = recipient
		}
		if _, err := e.db.Raw.Exec(e.db.Rebind(`INSERT INTO recurring_transactions (id, user_id, account_id, account_recipient_id, type, amount, description,
			schedule, next_payment_at, scheduled_day, created_at, updated_at) VALUES (?, ?, ?, ?, 2, '1', '', 'monthly', ?, 10, ?, ?)`),
			id, owner, account, rcpt, wipeDay, wipeDay, wipeDay); err != nil {
			t.Fatal(err)
		}
		return id
	}
	recurring := func(account, templateID string) string {
		id := row(account, fixture.Transaction{})
		if _, err := e.db.Raw.Exec(e.db.Rebind(`UPDATE transactions SET recurring_id = ? WHERE id = ?`), templateID, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ids = append(ids, recurring(accRecU, template(e.u, accRecU, "")))
	ids = append(ids, recurring(accRecP, template(e.p, accRecP, uAcc)))
	row(accCtl, fixture.Transaction{CategoryID: e.f.Category(fixture.Category{UserID: e.p})})

	before := map[string]string{}
	for _, id := range ids {
		before[id] = e.snapshot(id)
	}
	revs := e.f.Revisions(accCat, accPayee, accTag, accLabel, accImport, accRecU, accRecP, accCtl)

	e.rewrite()

	for _, id := range ids {
		if got := e.snapshot(id); got != before[id] {
			t.Errorf("row %s changed:\nbefore %s\nafter  %s", id, before[id], got)
		}
	}
	e.f.RequireRevisionsMoved(revs, accCat, accPayee, accTag, accLabel, accImport, accRecU, accRecP)
}

func TestRewrite_OwnTransfersUntouched(t *testing.T) {
	e := newWipeEnv(t)
	u1 := e.f.Account(fixture.Account{UserID: e.u})
	u2 := e.f.Account(fixture.Account{UserID: e.u})
	p1 := e.f.Account(fixture.Account{UserID: e.p})
	p2 := e.f.Account(fixture.Account{UserID: e.p})
	uOwn := e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: u1, AccountRecipientID: u2, Type: 2, Amount: "4", AmountRecipient: "4", SpentAt: wipeDay})
	pOwn := e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: p1, AccountRecipientID: p2, Type: 2, Amount: "6", AmountRecipient: "6", SpentAt: wipeDay})
	before := map[string]string{uOwn: e.snapshot(uOwn), pOwn: e.snapshot(pOwn)}
	revs := e.f.Revisions(u1, u2, p1, p2)

	e.rewrite()

	for id, snap := range before {
		if got := e.snapshot(id); got != snap {
			t.Errorf("own transfer %s changed:\nbefore %s\nafter  %s", id, snap, got)
		}
	}
	e.f.RequireRevisionsMoved(revs)
	e.requireUniqueKeys(u1, u2, p1, p2)
}

func TestRewrite_SortKeysStayUnique(t *testing.T) {
	e := newWipeEnv(t)
	pAcc := e.f.Account(fixture.Account{UserID: e.p})
	uAcc := e.f.Account(fixture.Account{UserID: e.u})
	e.neighbours(e.p, pAcc)
	e.f.Transaction(fixture.Transaction{UserID: e.p, AccountID: pAcc, AccountRecipientID: uAcc, Type: 2, Amount: "10", AmountRecipient: "10",
		SpentAt: wipeDay, SortKey: "m", RecipientSortKey: "m"})
	e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: uAcc, AccountRecipientID: pAcc, Type: 2, Amount: "2", AmountRecipient: "2",
		SpentAt: wipeDay, SortKey: "n", RecipientSortKey: "p"})
	e.f.Transaction(fixture.Transaction{UserID: e.u, AccountID: pAcc, Type: 0, Amount: "1", SpentAt: wipeDay, SortKey: "r"})
	bal := e.balances(pAcc)

	e.rewrite()

	if cs := e.correctionsOn(pAcc); len(cs) != 2 || cs[0].SortKey != "m" || cs[1].SortKey != "p" {
		t.Errorf("corrections = %+v", cs)
	}
	e.requireSameBalances(pAcc, bal)
	e.requireUniqueKeys(pAcc)
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}
