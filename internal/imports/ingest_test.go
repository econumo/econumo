package imports_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/imports/applewallet"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/imports/simplefin"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID  = "dffc2a06-6f29-4704-8575-31709adee926"
	userA  = "0a000000-0000-0000-0000-000000000001"
	userB  = "0a000000-0000-0000-0000-000000000002"
	acct1  = "0a000000-0000-0000-0000-0000000000a1"
	acctB  = "0a000000-0000-0000-0000-0000000000b1"
	source = "0c000000-0000-0000-0000-000000000001"
)

var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c clock) Now() time.Time { return c.t }

// fakeAccounts: acct1 (userA, USD, live), acctB (userB). deleted toggles acct1.
type fakeAccounts struct{ deleted bool }

func (f *fakeAccounts) AccountOwner(_ context.Context, id vo.Id) (vo.Id, error) {
	switch id.String() {
	case acct1:
		return vo.MustParseId(userA), nil
	case acctB:
		return vo.MustParseId(userB), nil
	}
	return vo.Id{}, errs.NewNotFound("Account not found")
}
func (f *fakeAccounts) AccountDeleted(_ context.Context, id vo.Id) (bool, error) {
	if id.String() != acct1 && id.String() != acctB {
		return false, errs.NewNotFound("Account not found")
	}
	return f.deleted && id.String() == acct1, nil
}
func (f *fakeAccounts) AccountCurrencyCode(_ context.Context, id vo.Id) (string, error) {
	return "USD", nil
}

// fakeConverter knows EUR->USD only.
type fakeConverter struct{ calls int }

func (f *fakeConverter) Convert(_ context.Context, _ vo.Id, from, to, amount string, _ time.Time) (string, bool, error) {
	f.calls++
	if from == "EUR" && to == "USD" {
		return vo.NewDecimal(amount).Mul(vo.NewDecimal("1.10")).String(), true, nil
	}
	return "", false, nil
}

// fakeTxns records creates and serves the candidate list. It also seeds a
// real transactions row for every create: import_transaction_links.transaction_id
// is a real FK (ON DELETE SET NULL), so the row it points at must exist. The
// insert goes through the TxManager bound to ctx (the pipeline's own
// transaction) rather than fixture.Builder, which opens a second connection
// on a pool capped at one — going through db.Raw here would deadlock against
// the outer WithTx.
type fakeTxns struct {
	db         *dbtest.DB
	builder    *fixture.Builder
	created    []model.CreateTransactionRequest
	updated    []model.UpdateTransactionRequest
	replaced   []model.UpdateTransactionRequest
	candidates []*model.Transaction
	fail       error
	// failOn, when non-empty, makes CreateTransaction fail for a request
	// whose Description matches it — used to test that one bad account in a
	// sync does not roll back the others.
	failOn string
}

func aliasToType(alias string) int {
	switch alias {
	case "income":
		return 1
	case "transfer":
		return 2
	default:
		return 0
	}
}

func (f *fakeTxns) CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	if f.failOn != "" && req.Description != nil && *req.Description == f.failOn {
		return nil, errors.New("create failed")
	}
	f.created = append(f.created, req)
	var description string
	if req.Description != nil {
		description = *req.Description
	}
	q := f.db.TX.Querier(ctx)
	query := f.db.Rebind(`INSERT INTO transactions (id, user_id, account_id, category_id, payee_id, tag_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if _, err := q.ExecContext(ctx, query,
		req.Id, userID.String(), req.AccountId, optID(req.CategoryId), optID(req.PayeeId), optID(req.TagId),
		aliasToType(req.Type), req.Amount.String(), description, req.Date, req.Date, req.Date); err != nil {
		return nil, err
	}
	if err := f.replaceLabels(ctx, req.Id, req.LabelIds); err != nil {
		return nil, err
	}
	return &model.CreateTransactionResult{Item: model.TransactionResult{Id: req.Id, AccountId: req.AccountId, Amount: req.Amount.String()}}, nil
}

// optID keeps a nil/blank optional id out of an FK column as NULL.
func optID(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

// replaceLabels mirrors the transaction feature's label rewrite so a later
// GetByID observes exactly the set the request carried.
func (f *fakeTxns) replaceLabels(ctx context.Context, transactionID string, labelIDs []string) error {
	q := f.db.TX.Querier(ctx)
	if _, err := q.ExecContext(ctx, f.db.Rebind(`DELETE FROM transactions_labels WHERE transaction_id = ?`), transactionID); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if _, err := q.ExecContext(ctx, f.db.Rebind(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`), transactionID, id); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeTxns) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error) {
	q := f.db.TX.Querier(ctx)
	var (
		userID, accountID, amount, description string
		typ                                    int
		cat, payee, tag                        *string
		spentAt                                time.Time
	)
	row := q.QueryRowContext(ctx, f.db.Rebind(
		`SELECT user_id, type, account_id, amount, category_id, payee_id, tag_id, description, spent_at FROM transactions WHERE id = ?`), id.String())
	if err := row.Scan(&userID, &typ, &accountID, &amount, &cat, &payee, &tag, &description, &spentAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Transaction not found")
		}
		return nil, err
	}
	t := &model.Transaction{
		ID: id, UserID: vo.MustParseId(userID), Type: model.TransactionType(typ),
		AccountID: vo.MustParseId(accountID), Amount: amount, Description: description, SpentAt: spentAt,
		CategoryID: parseOptID(cat), PayeeID: parseOptID(payee), TagID: parseOptID(tag),
	}
	rows, err := q.QueryContext(ctx, f.db.Rebind(`SELECT label_id FROM transactions_labels WHERE transaction_id = ? ORDER BY label_id`), id.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		t.LabelIDs = append(t.LabelIDs, vo.MustParseId(raw))
	}
	return t, rows.Err()
}

func parseOptID(p *string) *vo.Id {
	if p == nil || *p == "" {
		return nil
	}
	id := vo.MustParseId(*p)
	return &id
}

func (f *fakeTxns) UpdateTransactionReplacingLabels(ctx context.Context, _ vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	f.replaced = append(f.replaced, req)
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	q := f.db.TX.Querier(ctx)
	if _, err := q.ExecContext(ctx, f.db.Rebind(
		`UPDATE transactions SET category_id = ?, payee_id = ?, tag_id = ?, description = ? WHERE id = ?`),
		optID(req.CategoryId), optID(req.PayeeId), optID(req.TagId), description, req.Id); err != nil {
		return nil, err
	}
	if err := f.replaceLabels(ctx, req.Id, req.LabelIds); err != nil {
		return nil, err
	}
	return &model.UpdateTransactionResult{}, nil
}

func (f *fakeTxns) UpdateTransaction(_ context.Context, _ vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	f.updated = append(f.updated, req)
	return &model.UpdateTransactionResult{}, nil
}
func (f *fakeTxns) ListByAccount(_ context.Context, _ vo.Id, _, _ time.Time) ([]*model.Transaction, error) {
	return f.candidates, nil
}

// seed inserts a hand-entered transaction through the fixture builder (a
// real row, since import_transaction_links.transaction_id is a real FK),
// with a matching Apple Wallet tap link on the harness's default push
// source (the "source" constant, seeded by every setup(t)) — the matcher's
// tip-adopt path reads a candidate's push links straight from the DB via
// ListLinksByTransaction, so a tip-adopt test needs a real ledger row here,
// not just a fakeTxns.candidates entry — and registers it as a matcher
// candidate for ListByAccount.
func (f *fakeTxns) seed(t *testing.T, accountID, typeAlias, amount string, at time.Time, description string) vo.Id {
	t.Helper()
	id := vo.NewId()
	typ := aliasToType(typeAlias)
	f.builder.Transaction(fixture.Transaction{
		ID: id.String(), UserID: userA, AccountID: accountID, Type: typ, Amount: amount, Description: description, SpentAt: at,
	})
	f.builder.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: source, ExternalAccountID: accountID, ExternalTransactionID: "tap-" + id.String(),
		TransactionID: id.String(), Status: model.ImportLinkStatusLinked, ExternalPayee: description,
		ExternalAmount: amount, ExternalPostedAt: at,
	})
	f.candidates = append(f.candidates, &model.Transaction{
		ID: id, AccountID: vo.MustParseId(accountID), Type: model.TransactionType(typ), Amount: amount, SpentAt: at, Description: description,
	})
	return id
}

// fakeEntities is the owner's vocabulary as the rules engine sees it. Tests
// register ids they want a rule to be allowed to target.
type fakeEntities struct {
	categories, payees, tags, labels map[vo.Id][]model.ImportNamed // by owner
	// err, when set, is returned by CategoriesByOwner — used to simulate a
	// genuine lookup failure inside loadRules (as opposed to context
	// cancellation), which every caller must handle without leaving
	// half-finished state behind (e.g. a sync run stuck at "running").
	err error
}

func newFakeEntities() *fakeEntities {
	return &fakeEntities{
		categories: map[vo.Id][]model.ImportNamed{}, payees: map[vo.Id][]model.ImportNamed{},
		tags: map[vo.Id][]model.ImportNamed{}, labels: map[vo.Id][]model.ImportNamed{},
	}
}

func (f *fakeEntities) add(m map[vo.Id][]model.ImportNamed, owner vo.Id, id, name string) {
	m[owner] = append(m[owner], model.ImportNamed{ID: id, Name: name, OwnerID: owner.String()})
}

func (f *fakeEntities) CategoriesByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.categories[o], nil
}
func (f *fakeEntities) PayeesByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error) {
	return f.payees[o], nil
}
func (f *fakeEntities) TagsByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error) {
	return f.tags[o], nil
}
func (f *fakeEntities) LabelsByOwner(_ context.Context, o vo.Id) ([]model.ImportNamed, error) {
	return f.labels[o], nil
}

type limiter struct {
	allow, fail int
	deny        error
}

func (l *limiter) Allow(scope, key string) error {
	l.allow++
	if l.deny != nil {
		return l.deny
	}
	if l.allow > 2 {
		return errs.NewTooManyRequests("Too many attempts. Try again later.")
	}
	return nil
}
func (l *limiter) Fail(scope, key string) { l.fail++ }

type harness struct {
	svc      *imports.Service
	repo     *importsrepo.Repo
	accounts *fakeAccounts
	conv     *fakeConverter
	txns     *fakeTxns
	entities *fakeEntities
	lim      *limiter
	f        *fixture.Builder
	db       *dbtest.DB
}

// setup builds a harness with rate limiting disabled (nil limiter — see
// AttemptLimiter's doc: "a nil limiter disables protection (tests)"), so
// ordinary pipeline tests can make more than two ingest calls. Only
// TestIngest_RateLimited exercises the real fake limiter, via withLimiter.
func setup(t *testing.T) *harness {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db).At(now)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.User(fixture.User{ID: userB, Email: "b@example.test", Name: "B"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	f.Account(fixture.Account{ID: acctB, UserID: userB, CurrencyID: usdID, Name: "Other"})
	f.ImportSource(fixture.ImportSource{ID: source, UserID: userA, Name: "iPhone"})
	repo := importsrepo.NewRepo(db.Engine, db.TX)
	h := &harness{repo: repo, accounts: &fakeAccounts{}, conv: &fakeConverter{}, txns: &fakeTxns{db: db, builder: f}, entities: newFakeEntities(), lim: &limiter{}, f: f, db: db}
	h.svc = imports.NewService(repo, h.accounts, h.conv, h.txns, h.txns, h.entities, nil, db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
	return h
}

// withLimiter rebuilds the service with h.lim wired in as the rate limiter.
func (h *harness) withLimiter() {
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.entities, h.lim, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
}

// withRepo rebuilds the service over a decorated repository, so a single
// failing read can be exercised against otherwise real persistence.
func (h *harness) withRepo(repo imports.Repository) {
	h.svc = imports.NewService(repo, h.accounts, h.conv, h.txns, h.txns, h.entities, nil, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
	registerParsers(h.svc)
}

func registerParsers(svc *imports.Service) {
	svc.RegisterParser(model.ImportProviderAppleWallet, applewallet.Parser{})
	svc.RegisterParser(model.ImportProviderSimpleFIN, simplefin.Parser{})
}

func (h *harness) mapCard(t *testing.T, card string) {
	t.Helper()
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: card, AccountID: acct1})
}

func ingest(t *testing.T, h *harness, body string) *model.IngestEventResult {
	t.Helper()
	res, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userA), []byte(body))
	if err != nil {
		t.Fatalf("IngestAppleWallet: %v", err)
	}
	return res
}

const tap = `{"account":"Apple Card","payee":"Blue Bottle","amount":"4.75","currency":"USD","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"evt-1"}`

func TestIngest_UnmappedCardQueues(t *testing.T) {
	h := setup(t)
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusQueued || res.EventId == "" {
		t.Fatalf("res = %+v", res)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusQueued || links[0].ExternalPayee != "Blue Bottle" || links[0].ExternalCurrency == nil || *links[0].ExternalCurrency != "USD" {
		t.Fatalf("ledger = %+v", links)
	}
	if got := links[0].ExternalPostedAt.Format("2006-01-02 15:04:05"); got != "2026-08-20 10:42:03" {
		t.Errorf("posted_at must be the tap's wall clock, got %s", got)
	}
	if len(h.txns.created) != 0 {
		t.Error("nothing may be created for an unmapped card")
	}
	// a re-fired identical payload is a duplicate at the inbox
	if again := ingest(t, h, tap); again.Status != model.ImportIngestStatusDuplicate {
		t.Errorf("re-fire = %+v", again)
	}
	// same tap, different formatting: same external id -> still queued, no second row
	if again := ingest(t, h, `{"account":" apple  card","payee":"Blue Bottle","amount":"$4.75","currency":"usd","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"evt-1"}`); again.Status != model.ImportIngestStatusQueued {
		t.Errorf("reformatted = %+v", again)
	}
	if links, _ = h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source)); len(links) != 1 {
		t.Errorf("ledger must not fork: %d rows", len(links))
	}
}

func TestIngest_MappedCardCreatesTransaction(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "apple card") // case-insensitive lookup
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("res = %+v", res)
	}
	if len(h.txns.created) != 1 {
		t.Fatalf("created = %+v", h.txns.created)
	}
	req := h.txns.created[0]
	if req.AccountId != acct1 || req.Type != "expense" || req.Amount.String() != vo.NewDecimal("4.75").String() || req.Date != "2026-08-20 10:42:03" || req.Description == nil || *req.Description != "Blue Bottle" || req.Id == "" {
		t.Errorf("create request = %+v", req)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusLinked || links[0].TransactionID == nil {
		t.Fatalf("ledger = %+v", links)
	}
	// the exact key is now seen: re-delivery with a fresh payload hash is a duplicate at the ledger
	if again := ingest(t, h, `{"account":"Apple Card","payee":"Blue Bottle","amount":"4.75","currency":"USD","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"evt-1","type":"expense"}`); again.Status != model.ImportIngestStatusDuplicate {
		t.Errorf("re-delivery = %+v", again)
	}
}

func TestIngest_AdoptsHandEnteredTransaction(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	existing := vo.NewId()
	// import_transaction_links.transaction_id is a real FK, so the candidate
	// the matcher adopts must actually exist in transactions.
	h.f.Transaction(fixture.Transaction{ID: existing.String(), UserID: userA, AccountID: acct1, Type: 0, Amount: "4.75000000", SpentAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)})
	h.txns.candidates = []*model.Transaction{{ID: existing, AccountID: vo.MustParseId(acct1), Type: model.TransactionTypeExpense, Amount: "4.75000000", SpentAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)}}
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusMatched || len(h.txns.created) != 0 {
		t.Fatalf("res = %+v, created = %d", res, len(h.txns.created))
	}
	links, _ := h.repo.ListLinksByTransaction(context.Background(), existing)
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusLinked {
		t.Fatalf("adopt must link the existing transaction: %+v", links)
	}
}

const source2 = "0c000000-0000-0000-0000-000000000002"

// seedTipCandidate seeds a hand-entered transaction with a push (Apple
// Wallet) tap link, plus a second, pull-provider source ("bank") mapped to
// the same account. The bank event below reports the same purchase 2 days
// later at a corrected amount, within the tip tolerance — a tip-adopt.
func seedTipCandidate(t *testing.T, h *harness) (*model.ImportSource, vo.Id, model.IngestEvent) {
	t.Helper()
	h.f.ImportSource(fixture.ImportSource{ID: source2, UserID: userA, Name: "Bank", Provider: model.ImportProviderSimpleFIN})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source2, ExternalAccountID: "Apple Card", AccountID: acct1})
	cand := vo.NewId()
	spentAt := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	h.f.Transaction(fixture.Transaction{ID: cand.String(), UserID: userA, AccountID: acct1, Type: 0, Amount: "5.00000000", SpentAt: spentAt})
	h.f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: source, ExternalAccountID: "Apple Card", ExternalTransactionID: "tap-1",
		TransactionID: cand.String(), Status: "linked", ExternalPayee: "Blue Bottle Coffee",
		ExternalAmount: "5.00000000", ExternalPostedAt: spentAt,
	})
	h.txns.candidates = []*model.Transaction{{
		ID: cand, AccountID: vo.MustParseId(acct1), Type: model.TransactionTypeExpense,
		Amount: "5.00000000", SpentAt: spentAt, Description: "Blue Bottle tap",
	}}
	src2, err := h.repo.GetSource(context.Background(), vo.MustParseId(source2))
	if err != nil {
		t.Fatal(err)
	}
	ev := model.IngestEvent{
		ExternalAccountID: "Apple Card", ExternalTransactionID: "bank-1", Type: model.TransactionTypeExpense,
		Amount: "5.75", Currency: "USD", PostedAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		Payee: "Blue Bottle Coffee Shop",
	}
	return src2, cand, ev
}

func TestIngest_TipAdoptWithoutCorrection(t *testing.T) {
	h := setup(t)
	src2, cand, ev := seedTipCandidate(t, h)
	eventID := vo.MustParseId(h.f.ImportEvent(fixture.ImportEvent{SourceID: source2, Payload: "{}"}))
	status, amountUpdated, err := h.svc.ApplyEventForTest(context.Background(), src2, eventID, ev, nil, false)
	if err != nil || status != model.ImportIngestStatusMatched || amountUpdated {
		t.Fatalf("status=%s amountUpdated=%v err=%v", status, amountUpdated, err)
	}
	if len(h.txns.updated) != 0 {
		t.Fatalf("correctAmount=false must not update the candidate: %+v", h.txns.updated)
	}
	links, _ := h.repo.ListLinksByTransaction(context.Background(), cand)
	if len(links) != 2 {
		t.Fatalf("adopt must link the existing transaction a second time: %+v", links)
	}
}

func TestIngest_TipAdoptCorrectsAmount(t *testing.T) {
	h := setup(t)
	src2, cand, ev := seedTipCandidate(t, h)
	eventID := vo.MustParseId(h.f.ImportEvent(fixture.ImportEvent{SourceID: source2, Payload: "{}"}))
	status, amountUpdated, err := h.svc.ApplyEventForTest(context.Background(), src2, eventID, ev, nil, true)
	if err != nil || status != model.ImportIngestStatusMatched || !amountUpdated {
		t.Fatalf("status=%s amountUpdated=%v err=%v", status, amountUpdated, err)
	}
	if len(h.txns.updated) != 1 {
		t.Fatalf("correctAmount=true must update the candidate once: %+v", h.txns.updated)
	}
	got := h.txns.updated[0]
	if got.Id != cand.String() || !vo.NewDecimal(got.Amount.String()).Equals(vo.NewDecimal(ev.Amount)) {
		t.Fatalf("update request = %+v", got)
	}
}

func TestIngest_IgnoredCardSkips(t *testing.T) {
	h := setup(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", Mode: "ignore"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("res = %+v", res)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusSkipped {
		t.Fatalf("ledger = %+v", links)
	}
}

func TestIngest_DeletedAccountQueues(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.accounts.deleted = true
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("res = %+v", res)
	}
}

func TestIngest_CurrencyConversion(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	eur := `{"account":"Apple Card","payee":"Cafe","amount":"10","currency":"EUR","occurredAt":"2026-08-20T10:42:03+02:00","eventId":"e-eur"}`
	if res := ingest(t, h, eur); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("res = %+v", res)
	}
	if got := h.txns.created[0].Amount.String(); !vo.NewDecimal(got).Equals(vo.NewDecimal("11")) {
		t.Errorf("converted amount = %s", got)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if !vo.NewDecimal(links[0].ExternalAmount).Equals(vo.NewDecimal("10")) || *links[0].ExternalCurrency != "EUR" {
		t.Errorf("ledger must keep the original amount/currency: %+v", links[0])
	}
	// no rate -> queued, nothing created
	gbp := `{"account":"Apple Card","payee":"Pub","amount":"10","currency":"GBP","occurredAt":"2026-08-20T10:42:03+01:00","eventId":"e-gbp"}`
	if res := ingest(t, h, gbp); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("no-rate res = %+v", res)
	}
	if len(h.txns.created) != 1 {
		t.Errorf("no-rate tap must not create: %d", len(h.txns.created))
	}
}

func TestIngest_ParseFailureIsStoredAndReported(t *testing.T) {
	h := setup(t)
	res := ingest(t, h, `{"account":"Apple Card","amount":"nope","currency":"USD"}`)
	if res.Status != model.ImportIngestStatusFailed || res.EventId == "" {
		t.Fatalf("res = %+v", res)
	}
	ev, err := h.repo.GetEvent(context.Background(), vo.MustParseId(res.EventId))
	if err != nil || ev.Status != model.ImportEventStatusFailed || ev.ParseError == nil || *ev.ParseError != "amount must be a positive number" {
		t.Fatalf("event = %+v, %v", ev, err)
	}
	// not even JSON: still persisted, still 'failed'
	if res := ingest(t, h, `garbage`); res.Status != model.ImportIngestStatusFailed {
		t.Errorf("garbage = %+v", res)
	}
}

func TestIngest_NoSourceIsCoded400(t *testing.T) {
	h := setup(t)
	_, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userB), []byte(tap))
	verr, ok := errs.AsValidation(err)
	if !ok || verr.MsgCode != errs.CodeImportSourceNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestIngest_RateLimited(t *testing.T) {
	h := setup(t)
	h.withLimiter()
	ingest(t, h, tap)
	ingest(t, h, `{"account":"Apple Card","amount":"1","currency":"USD","eventId":"x"}`)
	_, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userA), []byte(tap))
	var tooMany *errs.TooManyRequestsError
	if !errors.As(err, &tooMany) {
		t.Fatalf("third call = %v, want 429", err)
	}
	if h.lim.fail != 2 {
		t.Errorf("every accepted request must count: fail=%d", h.lim.fail)
	}
}

func TestIngest_CreateFailureRollsBack(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.txns.fail = errors.New("boom")
	if _, err := h.svc.IngestAppleWallet(context.Background(), vo.MustParseId(userA), []byte(tap)); err == nil {
		t.Fatal("create failure must surface")
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	events, _ := h.repo.ListEventsBySourceStatus(context.Background(), vo.MustParseId(source), model.ImportEventStatusProcessed)
	if len(links) != 0 || len(events) != 0 {
		t.Errorf("one transaction: nothing may persist after a failed create (links=%d events=%d)", len(links), len(events))
	}
}

func TestRetryEvent(t *testing.T) {
	h := setup(t)
	failed := ingest(t, h, `{"account":"Apple Card","amount":"nope","currency":"USD"}`)
	// retry re-parses the stored payload; it is still broken, so it stays failed
	res, err := h.svc.RetryEvent(context.Background(), vo.MustParseId(userA), model.RetryImportEventRequest{EventId: failed.EventId})
	if err != nil || res.Status != model.ImportIngestStatusFailed {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	ok := ingest(t, h, tap)
	_, err = h.svc.RetryEvent(context.Background(), vo.MustParseId(userA), model.RetryImportEventRequest{EventId: ok.EventId})
	if verr, isV := errs.AsValidation(err); !isV || verr.MsgCode != errs.CodeImportEventNotFailed {
		t.Fatalf("retry of a processed event = %v", err)
	}
	if _, err := h.svc.RetryEvent(context.Background(), vo.MustParseId(userB), model.RetryImportEventRequest{EventId: failed.EventId}); err == nil {
		t.Fatal("foreign user must not retry")
	}
}

func TestIngest_SkipRuleSkipsAfterMapping(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "prefix", MatchValue: "payment - thank you"})
	body := strings.Replace(tap, `"payee":"Blue Bottle"`, `"payee":"PAYMENT - THANK YOU"`, 1)
	res := ingest(t, h, body)
	if res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("status = %s", res.Status)
	}
	if len(h.txns.created) != 0 {
		t.Fatalf("a skipped event must create nothing: %+v", h.txns.created)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusSkipped || links[0].AppliedRuleID == nil {
		t.Fatalf("ledger row must be skipped with the rule recorded: %+v", links)
	}
}

func TestIngest_SkipRuleDoesNotFireOnUnmappedCard(t *testing.T) {
	h := setup(t)
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	res := ingest(t, h, tap) // card never mapped
	if res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("unmapped card queues even when a skip rule would match: %s", res.Status)
	}
}

func TestIngest_SkipRuleScopedToAnotherSourceIsIgnored(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	other := h.f.ImportSource(fixture.ImportSource{UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank"})
	h.f.ImportRule(fixture.ImportRule{UserID: userA, SourceID: other, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("rule scoped to another source must not fire: %s", res.Status)
	}
}

func TestLinkAccount_ReplayAppliesSkipRules(t *testing.T) {
	h := setup(t)
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatalf("queued first: %s", res.Status)
	}
	res, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run == nil || res.Run.SkippedCount != 1 || res.Run.ImportedCount != 0 {
		t.Fatalf("replay must skip via the rule: %+v", res.Run)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if links[0].Status != model.ImportLinkStatusSkipped || links[0].AppliedRuleID == nil {
		t.Fatalf("replayed row: %+v", links[0])
	}
}

func TestIngest_ClassifyRuleFillsCreatedTransaction(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Work"})
	// a payee the owner does not have (deleted after the rule was saved); it is
	// a real row because import_rules.target_payee_id is an FK, but it belongs
	// to another user, so it is outside userA's vocabulary
	stale := h.f.Payee(fixture.Payee{UserID: userB, Name: "Gone"})
	h.entities.add(h.entities.categories, vo.MustParseId(userA), cat, "Coffee")
	h.entities.add(h.entities.labels, vo.MustParseId(userA), label, "Work")
	ruleID := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat, PayeeID: stale, LabelIDs: []string{label}})
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	req := h.txns.created[0]
	if req.CategoryId == nil || *req.CategoryId != cat || req.PayeeId != nil || len(req.LabelIds) != 1 || req.LabelIds[0] != label {
		t.Fatalf("created request must carry the rule's live targets only: %+v", req)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	l := links[0]
	if l.AppliedCategoryID == nil || l.AppliedCategoryID.String() != cat || l.AppliedRuleID == nil || l.AppliedRuleID.String() != ruleID {
		t.Fatalf("applied snapshot: %+v", l)
	}
	applied, _ := h.repo.ListLinkAppliedLabels(context.Background(), l.ID)
	if len(applied) != 1 || applied[0].String() != label {
		t.Fatalf("applied labels: %v", applied)
	}
}

func TestIngest_AdoptedTransactionSnapshotsItsOwnClassification(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	// a classify rule that matches this event and would have written another
	// category had the event created a transaction
	other := h.f.Category(fixture.Category{UserID: userA, Name: "Other"})
	h.entities.add(h.entities.categories, vo.MustParseId(userA), other, "Other")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: other})
	// A hand-entered transaction the matcher will adopt (same amount, same
	// day), seeded without a ledger row of its own: Match never adopts a
	// candidate this source already links, so h.txns.seed's tap link would
	// turn the adopt into a create.
	txID := vo.NewId()
	h.f.Transaction(fixture.Transaction{ID: txID.String(), UserID: userA, AccountID: acct1, CategoryID: cat, Amount: "4.75", Description: "Blue Bottle", SpentAt: now})
	h.txns.candidates = []*model.Transaction{{
		ID: txID, AccountID: vo.MustParseId(acct1), Type: model.TransactionTypeExpense,
		Amount: "4.75", SpentAt: now, Description: "Blue Bottle",
	}}
	res := ingest(t, h, tap)
	if res.Status != model.ImportIngestStatusMatched {
		t.Fatalf("status = %s", res.Status)
	}
	if len(h.txns.created) != 0 {
		t.Fatalf("an adopted event creates nothing: %+v", h.txns.created)
	}
	l := linkByExternalID(t, h, "evt-1")
	if l.AppliedCategoryID == nil || l.AppliedCategoryID.String() != cat || l.AppliedRuleID != nil {
		t.Fatalf("adopted row snapshots the transaction's current classification, not the rule's: %+v", l)
	}
}

// A skip rule's id lives on the same column as a classify rule's. Unskipping
// a row and importing it by hand must clear it: appliedRuleId is read as "the
// rule that classified this row", and a skip rule classified nothing.
func TestImportQueuedEvent_AfterUnskipDropsTheSkipRule(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	uA := vo.MustParseId(userA)
	h.mapCard(t, "Apple Card")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: "skip", MatchField: "external_payee", MatchType: "contains", MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("status = %s", res.Status)
	}
	links, _ := h.repo.ListLinksBySource(ctx, vo.MustParseId(source))
	linkID := links[0].ID
	if links[0].AppliedRuleID == nil {
		t.Fatalf("the skip rule must be recorded first: %+v", links[0])
	}
	if _, err := h.svc.UnskipQueuedEvent(ctx, uA, model.ImportLinkActionRequest{LinkId: linkID.String()}); err != nil {
		t.Fatalf("UnskipQueuedEvent: %v", err)
	}
	txID := vo.NewId().String()
	if _, err := h.svc.ImportQueuedEvent(ctx, uA, model.ImportQueuedEventRequest{
		LinkId:      linkID.String(),
		Transaction: model.CreateTransactionRequest{Id: txID, Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: now.Format(datetime.Layout)},
	}); err != nil {
		t.Fatalf("ImportQueuedEvent: %v", err)
	}
	link, _ := h.repo.GetLink(ctx, linkID)
	if link.AppliedRuleID != nil {
		t.Fatalf("a skip rule must not survive as the row's applied rule: %+v", link)
	}
	list, _ := h.svc.GetTransactionImportList(ctx, uA, model.TransactionImportListRequest{TransactionId: txID})
	if len(list.Items) != 1 || list.Items[0].AppliedRuleId != "" {
		t.Fatalf("provenance must report no applied rule: %+v", list.Items)
	}
}

func TestImportQueuedEvent_RecordsLabels(t *testing.T) {
	h := setup(t)
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Trip"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusQueued {
		t.Fatal("expected queued")
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	_, err := h.svc.ImportQueuedEvent(context.Background(), vo.MustParseId(userA), model.ImportQueuedEventRequest{
		LinkId:      links[0].ID.String(),
		Transaction: model.CreateTransactionRequest{Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("4.75"), AccountId: acct1, Date: now.Format(datetime.Layout), LabelIds: []string{label}},
	})
	if err != nil {
		t.Fatalf("ImportQueuedEvent: %v", err)
	}
	applied, _ := h.repo.ListLinkAppliedLabels(context.Background(), links[0].ID)
	if len(applied) != 1 || applied[0].String() != label {
		t.Fatalf("applied labels: %v", applied)
	}
	list, _ := h.svc.GetTransactionImportList(context.Background(), vo.MustParseId(userA), model.TransactionImportListRequest{TransactionId: h.txns.created[0].Id})
	if len(list.Items) != 1 || len(list.Items[0].AppliedLabelIds) != 1 || list.Items[0].AppliedLabelIds[0] != label {
		t.Fatalf("provenance must expose applied labels: %+v", list.Items)
	}
}

func linkByExternalID(t *testing.T, h *harness, externalTxID string) model.ImportTransactionLink {
	t.Helper()
	links, err := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if l.ExternalTransactionID == externalTxID {
			return l
		}
	}
	t.Fatalf("no link for %s in %+v", externalTxID, links)
	return model.ImportTransactionLink{}
}
