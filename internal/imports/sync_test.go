package imports_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const bankSource = "0c000000-0000-0000-0000-00000000000b"

type fakeProvider struct {
	accounts []model.ExternalAccount
	txs      []model.ExternalTransaction
	warnings []string
	err      error
	seen     []imports.Credential
	claimed  string
	onFetch  func()
}

func (p *fakeProvider) ListAccounts(_ context.Context, cred imports.Credential) ([]model.ExternalAccount, error) {
	p.seen = append(p.seen, cred)
	return p.accounts, p.err
}

func (p *fakeProvider) FetchTransactions(_ context.Context, cred imports.Credential, _ imports.FetchRequest) (*imports.FetchResult, error) {
	p.seen = append(p.seen, cred)
	if p.onFetch != nil {
		p.onFetch()
	}
	if p.err != nil {
		return nil, p.err
	}
	return &imports.FetchResult{Accounts: p.accounts, Transactions: p.txs, Warnings: p.warnings}, nil
}

func (p *fakeProvider) ClaimSetupToken(_ context.Context, tok string) (string, error) {
	p.claimed = tok
	if tok == "used" {
		return "", imports.ErrSetupTokenRejected
	}
	return "https://u:p@bridge.example/simplefin", nil
}

func extTx(account, id, amount string, posted int64, payee string) model.ExternalTransaction {
	raw, _ := json.Marshal(map[string]any{"id": id, "posted": posted, "amount": amount, "payee": payee, "description": payee})
	return model.ExternalTransaction{ExternalAccountID: account, ID: id, Amount: amount, Posted: posted, Payee: payee, Description: payee, Raw: raw}
}

func bankHarness(t *testing.T) (*harness, *fakeProvider) {
	t.Helper()
	h := setup(t)
	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	p := &fakeProvider{accounts: []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "10", OrgName: "Big Bank"}}}
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	return h, p
}

func syncReq(over ...func(*model.SyncImportSourceRequest)) model.SyncImportSourceRequest {
	r := model.SyncImportSourceRequest{SourceId: bankSource, AccessUrl: "https://u:p@bridge.example/simplefin", StartDate: "2026-08-01", EndDate: "2026-08-31"}
	for _, f := range over {
		f(&r)
	}
	return r
}

func TestSync_MappedAccountImportsAndCounts(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{
		extTx("ACT-1", "T1", "-12.50", 1755900000, "Coffee"),
		extTx("ACT-1", "T2", "2500", 1755910000, "Payroll"),
		extTx("ACT-1", "T3", "abc", 1755920000, "Broken"),
	}
	ctx := reqctx.WithLogAttrs(context.Background())
	res, err := h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())
	if err != nil {
		t.Fatal(err)
	}
	// One row failed to parse, so the run is partial even though no account failed.
	if res.Run.Status != model.ImportRunStatusPartial || res.Run.ImportedCount != 2 || res.Run.FailedCount != 1 || res.Run.Trigger != model.ImportRunTriggerManual {
		t.Fatalf("run = %+v", res.Run)
	}
	if len(res.Accounts) != 1 || res.Accounts[0].State != model.ImportCardStateMapped || res.Accounts[0].AccountId != acct1 || res.Accounts[0].OrgName != "Big Bank" {
		t.Fatalf("accounts = %+v", res.Accounts)
	}
	if len(h.txns.created) != 2 || h.txns.created[0].Type != "expense" || h.txns.created[0].Amount.String() != "12.5" || h.txns.created[1].Type != "income" {
		t.Fatalf("created = %+v", h.txns.created)
	}
	// second sync: the same rows are duplicates and nothing is counted
	res, err = h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())
	// The broken row's event was stored too (status failed), so on the second
	// pass every row — including it — is a payload duplicate and counts nothing.
	if err != nil || res.Run.ImportedCount != 0 || res.Run.FailedCount != 0 || res.Run.Status != model.ImportRunStatusCompleted {
		t.Fatalf("second run = %+v err %v", res.Run, err)
	}
	src, _ := h.repo.GetSource(ctx, vo.MustParseId(bankSource))
	if src.LastSyncedAt == nil {
		t.Fatal("last_synced_at must be set after a completed run")
	}
	assertNoAccessURLLeak(t, ctx, h, res)
}

// assertNoAccessURLLeak checks every surface the access URL (or its
// userinfo component) must never reach: every log attr accumulated on ctx
// (stringified, not just string-typed values — a leak via a wrapped/struct
// value would otherwise slip past a type assertion), every run error
// message, and every stored import_runs.params row for the bank source.
func assertNoAccessURLLeak(t *testing.T, ctx context.Context, h *harness, res *model.SyncImportSourceResult) {
	t.Helper()
	secrets := []string{syncReq().AccessUrl, "u:p"}
	leaks := func(s string) bool {
		for _, secret := range secrets {
			if strings.Contains(s, secret) {
				return true
			}
		}
		return false
	}
	for _, a := range reqctx.LogAttrs(ctx) {
		if s := fmt.Sprint(a.Value.Any()); leaks(s) {
			t.Fatalf("access url leaked into log attrs: %v", a)
		}
	}
	for i, e := range res.Run.Errors {
		if leaks(e.Message) {
			t.Fatalf("access url leaked into run error[%d]: %+v", i, e)
		}
	}
	rows, err := h.db.Raw.QueryContext(ctx, h.db.Rebind(`SELECT params FROM import_runs WHERE source_id = ?`), bankSource)
	if err != nil {
		t.Fatalf("read params: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var params string
		if err := rows.Scan(&params); err != nil {
			t.Fatalf("scan params: %v", err)
		}
		if leaks(params) {
			t.Fatalf("access url leaked into stored params: %s", params)
		}
	}
}

func TestSync_UnmappedAccountQueues(t *testing.T) {
	h, p := bankHarness(t)
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-1", 1755900000, "X")}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.QueuedCount != 1 || res.Run.ImportedCount != 0 || res.Accounts[0].State != model.ImportCardStateUnmapped {
		t.Fatalf("res = %+v err %v", res, err)
	}
}

func TestSync_ProviderWarningsMakeRunPartial(t *testing.T) {
	h, p := bankHarness(t)
	p.warnings = []string{"Connection to Big Bank may need attention"}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.Status != model.ImportRunStatusPartial || len(res.Run.Errors) != 1 || res.Run.Errors[0].ExternalAccountId != "" || res.Run.Errors[0].Message != p.warnings[0] {
		t.Fatalf("res = %+v err %v", res.Run, err)
	}
}

func TestSync_ProviderDownIsCoded(t *testing.T) {
	h, p := bankHarness(t)
	p.err = imports.ErrProviderUnavailable
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	verr, ok := errs.AsValidation(err)
	if !ok || verr.MsgCode != errs.CodeImportProviderUnavailable {
		t.Fatalf("err = %v", err)
	}
	runs, _ := h.repo.ListRunsByUser(context.Background(), vo.MustParseId(userA), nil, 10)
	if len(runs) != 1 || runs[0].Status != model.ImportRunStatusFailed || runs[0].FinishedAt == nil {
		t.Fatalf("a failed fetch must leave a failed run: %+v", runs)
	}
}

func TestSync_BadCredentialIsCoded(t *testing.T) {
	h, p := bankHarness(t)
	p.err = imports.ErrCredentialInvalid
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportAccessUrlInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestSync_RangeRules(t *testing.T) {
	h, _ := bankHarness(t)
	for _, r := range []model.SyncImportSourceRequest{
		syncReq(func(r *model.SyncImportSourceRequest) { r.StartDate, r.EndDate = "2026-08-31", "2026-08-01" }),
		syncReq(func(r *model.SyncImportSourceRequest) { r.StartDate, r.EndDate = "2025-01-01", "2026-08-01" }),
	} {
		_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), r)
		if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportSyncRangeInvalid {
			t.Fatalf("%+v: err = %v", r, err)
		}
	}
	// end date defaults to today (clock), start may equal end
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq(func(r *model.SyncImportSourceRequest) { r.EndDate = "" }))
	if err != nil || res.Run.Status != model.ImportRunStatusCompleted {
		t.Fatalf("res = %+v err %v", res, err)
	}
}

func TestSync_PushProviderAndForeignSourceRejected(t *testing.T) {
	h, _ := bankHarness(t)
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq(func(r *model.SyncImportSourceRequest) { r.SourceId = source }))
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportProviderUnsupported {
		t.Fatalf("push provider: %v", err)
	}
	_, err = h.svc.Sync(context.Background(), vo.MustParseId(userB), syncReq())
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("foreign source: %v", err)
	}
}

func TestSync_PerAccountFailureIsIsolated(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.accounts = append(p.accounts, model.ExternalAccount{ID: "ACT-2", Name: "Savings", Currency: "USD"})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-2", AccountID: acct1})
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-1", 1755900000, "ok"), extTx("ACT-2", "T2", "-2", 1755900000, "boom")}
	h.txns.failOn = "boom" // the fake creator errors on this payee
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.Status != model.ImportRunStatusPartial || res.Run.ImportedCount != 1 || len(res.Run.Errors) != 1 || res.Run.Errors[0].ExternalAccountId != "ACT-2" {
		t.Fatalf("res = %+v err %v", res.Run, err)
	}
	if res.Run.Errors[0].Message != "Import failed for this account" {
		t.Fatalf("message must be static, got %q", res.Run.Errors[0].Message)
	}
}

func TestSync_RateLimited(t *testing.T) {
	h, _ := bankHarness(t)
	h.withLimiter()
	h.lim.deny = errors.New("limited")
	_, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if !errors.Is(err, h.lim.deny) {
		t.Fatalf("expected the limiter's own error, got %v", err)
	}
}

func TestSync_TipAdoptCorrectsAmount(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	// hand-entered 10.00 on 2025-08-22, bank posts 11.50 (a 15% tip) two days later
	h.txns.seed(t, acct1, "expense", "10", time.Date(2025, 8, 22, 12, 0, 0, 0, time.UTC), "Coffee")
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-11.50", time.Date(2025, 8, 24, 12, 0, 0, 0, time.UTC).Unix(), "Coffee")}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq(func(r *model.SyncImportSourceRequest) { r.StartDate, r.EndDate = "2025-08-01", "2025-08-31" }))
	if err != nil || res.Run.MatchedCount != 1 || res.Run.AmountsUpdatedCount != 1 || res.Run.ImportedCount != 0 {
		t.Fatalf("res = %+v err %v", res.Run, err)
	}
	if len(h.txns.updated) != 1 || h.txns.updated[0].Amount.String() != "11.5" {
		t.Fatalf("updated = %+v", h.txns.updated)
	}
}

// A browser that navigates away mid-sync cancels the request context. The run
// row already exists by then, so it has to be finalized anyway — a row left at
// "running" is shown forever with no action able to clear it.
func TestSync_FinalizesRunAfterClientDisconnect(t *testing.T) {
	h, p := bankHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-12.50", 1755900000, "Coffee")}
	ctx, cancel := context.WithCancel(reqctx.WithLogAttrs(context.Background()))
	defer cancel()
	p.onFetch = cancel
	_, _ = h.svc.Sync(ctx, vo.MustParseId(userA), syncReq())

	runs, err := h.repo.ListRunsByUser(context.Background(), vo.MustParseId(userA), nil, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %+v err %v", runs, err)
	}
	if runs[0].Status != model.ImportRunStatusCompleted || runs[0].FinishedAt == nil || runs[0].ImportedCount != 1 {
		t.Fatalf("run = %+v", runs[0])
	}
	src, err := h.repo.GetSource(context.Background(), vo.MustParseId(bankSource))
	if err != nil || src.LastSyncedAt == nil {
		t.Fatalf("last_synced_at must be written too: %+v err %v", src, err)
	}
}
