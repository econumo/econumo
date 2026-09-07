package imports_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
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
	created    []model.CreateTransactionRequest
	candidates []*model.Transaction
	fail       error
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
	f.created = append(f.created, req)
	var description string
	if req.Description != nil {
		description = *req.Description
	}
	q := f.db.TX.Querier(ctx)
	query := f.db.Rebind(`INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if _, err := q.ExecContext(ctx, query,
		req.Id, userID.String(), req.AccountId, aliasToType(req.Type), req.Amount.String(), description, req.Date, req.Date, req.Date); err != nil {
		return nil, err
	}
	return &model.CreateTransactionResult{Item: model.TransactionResult{Id: req.Id, AccountId: req.AccountId, Amount: req.Amount.String()}}, nil
}
func (f *fakeTxns) ListByAccount(_ context.Context, _ vo.Id, _, _ time.Time) ([]*model.Transaction, error) {
	return f.candidates, nil
}

type limiter struct{ allow, fail int }

func (l *limiter) Allow(scope, key string) error {
	l.allow++
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
	h := &harness{repo: repo, accounts: &fakeAccounts{}, conv: &fakeConverter{}, txns: &fakeTxns{db: db}, lim: &limiter{}, f: f, db: db}
	h.svc = imports.NewService(repo, h.accounts, h.conv, h.txns, h.txns, nil, db.TX, clock{now}, imports.DefaultMatcherConfig())
	return h
}

// withLimiter rebuilds the service with h.lim wired in as the rate limiter.
func (h *harness) withLimiter() {
	h.svc = imports.NewService(h.repo, h.accounts, h.conv, h.txns, h.txns, h.lim, h.db.TX, clock{now}, imports.DefaultMatcherConfig())
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
	if res.Status != model.ImportIngestStatusCreated || len(h.txns.created) != 0 {
		t.Fatalf("res = %+v, created = %d", res, len(h.txns.created))
	}
	links, _ := h.repo.ListLinksByTransaction(context.Background(), existing)
	if len(links) != 1 || links[0].Status != model.ImportLinkStatusLinked {
		t.Fatalf("adopt must link the existing transaction: %+v", links)
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
