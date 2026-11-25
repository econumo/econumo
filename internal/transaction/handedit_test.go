package transaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

type recordingReclassifications struct {
	ids []string
	err error
}

func (r *recordingReclassifications) ReclassifiedByHand(_ context.Context, transactionID vo.Id) error {
	r.ids = append(r.ids, transactionID.String())
	return r.err
}

type handEditFixture struct {
	svc                  *apptransaction.Service
	rec                  *recordingReclassifications
	uid                  vo.Id
	account, other       string
	category, category2  string
	payee, payee2, tagID string
}

func newHandEditFixture(t *testing.T) handEditFixture {
	t.Helper()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	h := handEditFixture{
		svc:       newWriteService(t, db),
		rec:       &recordingReclassifications{},
		uid:       mustID(t, userID),
		account:   f.Account(fixture.Account{UserID: userID}),
		other:     f.Account(fixture.Account{UserID: userID}),
		category:  f.Category(fixture.Category{UserID: userID, Type: 0}),
		category2: f.Category(fixture.Category{UserID: userID, Type: 0}),
		payee:     f.Payee(fixture.Payee{UserID: userID}),
		payee2:    f.Payee(fixture.Payee{UserID: userID}),
		tagID:     f.Tag(fixture.Tag{UserID: userID}),
	}
	h.svc.SetReclassificationListener(h.rec)
	return h
}

func (h handEditFixture) create(t *testing.T) model.UpdateTransactionRequest {
	t.Helper()
	description := "coffee"
	req := model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("10.00"), AccountId: h.account,
		CategoryId: &h.category, PayeeId: &h.payee, Date: "2024-04-01 00:00:00", Description: &description,
	}
	res, err := h.svc.CreateTransaction(context.Background(), h.uid, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return model.UpdateTransactionRequest{
		Id: res.Item.Id, Type: req.Type, Amount: req.Amount, AccountId: req.AccountId,
		CategoryId: req.CategoryId, PayeeId: req.PayeeId, Date: req.Date, Description: req.Description,
	}
}

func strp(s string) *string { return &s }

// A hand edit is a category or payee change made through update-transaction
// or the MCP tools; amount, date, description and tag edits are not.
func TestUpdateTransactionReportsAHandReclassification(t *testing.T) {
	type update func(*apptransaction.Service, context.Context, vo.Id, model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error)
	plain := (*apptransaction.Service).UpdateTransaction
	preserving := (*apptransaction.Service).UpdateTransactionPreservingLabels
	automated := (*apptransaction.Service).AutomatedUpdateTransaction
	automatedPreserving := (*apptransaction.Service).AutomatedUpdateTransactionPreservingLabels
	for _, tc := range []struct {
		name   string
		call   update
		edit   func(h handEditFixture, req *model.UpdateTransactionRequest)
		report bool
	}{
		{"category", plain, func(h handEditFixture, r *model.UpdateTransactionRequest) { r.CategoryId = &h.category2 }, true},
		{"payee keeping labels", preserving, func(h handEditFixture, r *model.UpdateTransactionRequest) { r.PayeeId = &h.payee2 }, true},
		{"payee cleared", plain, func(h handEditFixture, r *model.UpdateTransactionRequest) { r.PayeeId = nil }, true},
		{"made a transfer", plain, func(h handEditFixture, r *model.UpdateTransactionRequest) {
			r.Type, r.CategoryId, r.PayeeId, r.AccountRecipientId = "transfer", nil, nil, &h.other
		}, true},
		{"description, amount, date and tag", plain, func(h handEditFixture, r *model.UpdateTransactionRequest) {
			r.Description, r.Amount, r.Date, r.TagId = strp("tea"), vo.NewFlexString("12.00"), "2024-04-02 00:00:00", &h.tagID
		}, false},
		{"category by the importer", automated, func(h handEditFixture, r *model.UpdateTransactionRequest) { r.CategoryId = &h.category2 }, false},
		{"payee by the importer keeping labels", automatedPreserving, func(h handEditFixture, r *model.UpdateTransactionRequest) { r.PayeeId = &h.payee2 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandEditFixture(t)
			req := h.create(t)
			tc.edit(h, &req)
			if _, err := tc.call(h.svc, context.Background(), h.uid, req); err != nil {
				t.Fatalf("update: %v", err)
			}
			want := 0
			if tc.report {
				want = 1
			}
			if len(h.rec.ids) != want || (want == 1 && h.rec.ids[0] != req.Id) {
				t.Fatalf("reported = %v, want %d report(s) for %s", h.rec.ids, want, req.Id)
			}
		})
	}
}

// The report runs inside the edit's transaction: when it fails, the edit does
// not land.
func TestUpdateTransactionRollsBackWhenTheReportFails(t *testing.T) {
	h := newHandEditFixture(t)
	req := h.create(t)
	h.rec.err = errors.New("boom")
	req.CategoryId = &h.category2
	if _, err := h.svc.UpdateTransaction(context.Background(), h.uid, req); err == nil {
		t.Fatal("update succeeded although the report failed")
	}
	list, err := h.svc.GetTransactionList(context.Background(), h.uid, model.TransactionListRequest{AccountId: h.account})
	if err != nil || len(list.Items) != 1 || list.Items[0].CategoryId == nil || *list.Items[0].CategoryId != h.category {
		t.Fatalf("after a failed report: %+v, %v; want the original category", list, err)
	}
}

func TestUpdateTransactionWithoutAListener(t *testing.T) {
	h := newHandEditFixture(t)
	h.svc.SetReclassificationListener(nil)
	req := h.create(t)
	req.CategoryId = &h.category2
	if _, err := h.svc.UpdateTransaction(context.Background(), h.uid, req); err != nil {
		t.Fatalf("update without a listener: %v", err)
	}
}

// Bulk reports each transaction whose category or payee it changed, and only
// those.
func TestBulkUpdateTransactionsReportsHandReclassifications(t *testing.T) {
	for _, tc := range []struct {
		name   string
		req    func(h handEditFixture) model.BulkUpdateTransactionsRequest
		report bool
	}{
		{"category", func(h handEditFixture) model.BulkUpdateTransactionsRequest {
			return model.BulkUpdateTransactionsRequest{CategoryId: &h.category2}
		}, true},
		{"payee cleared", func(h handEditFixture) model.BulkUpdateTransactionsRequest {
			return model.BulkUpdateTransactionsRequest{ClearPayee: true}
		}, true},
		{"same category and payee", func(h handEditFixture) model.BulkUpdateTransactionsRequest {
			return model.BulkUpdateTransactionsRequest{CategoryId: &h.category, PayeeId: &h.payee}
		}, false},
		{"tag only", func(h handEditFixture) model.BulkUpdateTransactionsRequest {
			return model.BulkUpdateTransactionsRequest{TagId: &h.tagID}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandEditFixture(t)
			a, b := h.create(t), h.create(t)
			req := tc.req(h)
			req.Ids = []string{a.Id, b.Id}
			if _, err := h.svc.BulkUpdateTransactions(context.Background(), h.uid, req); err != nil {
				t.Fatalf("bulk: %v", err)
			}
			if !tc.report {
				if len(h.rec.ids) != 0 {
					t.Fatalf("reported = %v, want none", h.rec.ids)
				}
				return
			}
			if len(h.rec.ids) != 2 || h.rec.ids[0] != a.Id || h.rec.ids[1] != b.Id {
				t.Fatalf("reported = %v, want [%s %s]", h.rec.ids, a.Id, b.Id)
			}
		})
	}
}

// orderRepo records the calls that matter for the hand-edit row lock.
type orderRepo struct {
	apptransaction.Repository
	calls []string
}

func (r *orderRepo) LockRow(ctx context.Context, id vo.Id) error {
	r.calls = append(r.calls, "LockRow "+id.String())
	return r.Repository.LockRow(ctx, id)
}

func (r *orderRepo) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error) {
	r.calls = append(r.calls, "GetByID "+id.String())
	return r.Repository.GetByID(ctx, id)
}

// A hand edit locks the transaction row before it reads it, so the category
// and payee it compares are the ones a concurrent writer (the import AI job)
// committed. The race itself needs two connections (handedit_pgsql_test.go);
// this is the engine-independent guard on the order.
func TestHandEditsLockTheRowBeforeReadingIt(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	account := f.Account(fixture.Account{UserID: userID})
	category := f.Category(fixture.Category{UserID: userID, Type: 0})
	rec := &orderRepo{}
	svc := newWriteServiceWithRepo(t, db, func(r apptransaction.Repository) apptransaction.Repository {
		rec.Repository = r
		return rec
	})
	uid := mustID(t, userID)
	a := createExpense(t, svc, uid, account, category, "2024-04-01")
	b := createExpense(t, svc, uid, account, category, "2024-04-02")
	req := model.UpdateTransactionRequest{Id: a, Type: "expense", Amount: vo.NewFlexString("10.00"), AccountId: account, Date: "2024-04-01 00:00:00"}

	for _, tc := range []struct {
		name string
		run  func() error
		want []string
	}{
		{"update", func() error { _, err := svc.UpdateTransaction(context.Background(), uid, req); return err },
			[]string{"LockRow " + a, "GetByID " + a}},
		{"update keeping labels", func() error {
			_, err := svc.UpdateTransactionPreservingLabels(context.Background(), uid, req)
			return err
		}, []string{"LockRow " + a, "GetByID " + a}},
		{"bulk", func() error {
			_, err := svc.BulkUpdateTransactions(context.Background(), uid, model.BulkUpdateTransactionsRequest{Ids: []string{b, a}, ClearCategory: true})
			return err
		}, sortedLocksThenReads(a, b)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec.calls = nil
			if err := tc.run(); err != nil {
				t.Fatal(err)
			}
			if len(rec.calls) < len(tc.want) {
				t.Fatalf("calls = %v, want %v first", rec.calls, tc.want)
			}
			for i, w := range tc.want {
				if rec.calls[i] != w {
					t.Fatalf("calls = %v, want %v first", rec.calls, tc.want)
				}
			}
		})
	}
}

// sortedLocksThenReads is bulk's order for ids sent as [b, a]: every row is
// locked in id order before any is read, so two overlapping bulk edits cannot
// deadlock.
func sortedLocksThenReads(a, b string) []string {
	lo, hi := a, b
	if hi < lo {
		lo, hi = hi, lo
	}
	return []string{"LockRow " + lo, "LockRow " + hi, "GetByID " + b, "GetByID " + a}
}
