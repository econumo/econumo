package transaction

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// stubImportLabelOwner is a minimal Importer whose only interesting behavior
// is label ownership scoping: AccountByID hands back a caller-supplied
// account (so the test controls its OwnerID directly, unlike the real
// production adapter which only ever resolves a caller's OWN account for an
// accountId override — that "own accounts only" business rule is orthogonal
// to what's under test here, which is whether the SERVICE threads the
// resolved account owner, not the caller, into the label ports).
// LabelsByOwner/CreateLabel record which owner id they were called with, and
// LabelsByOwner's return is keyed by that owner id so a wrong-owner call is
// observable both as a recorded call arg and as a different (wrong) result
// set flowing back into the labelIds belongs-to check.
type stubImportLabelOwner struct {
	account *model.ImportAccount

	labelsByOwner      map[string][]model.ImportNamed
	labelsByOwnerCalls []vo.Id

	nextLabelID      vo.Id
	createLabelCalls []struct {
		ownerID vo.Id
		name    string
	}
	// createLabelErrFor simulates the real CreateLabel port's downstream
	// validation (internal/label's name-length rule): a name present here
	// fails instead of resolving to nextLabelID, letting tests pin that the
	// import row surfaces the error rather than swallowing it.
	createLabelErrFor map[string]error
}

func (s *stubImportLabelOwner) AvailableAccounts(ctx context.Context, userID vo.Id) ([]model.ImportAccount, error) {
	return nil, nil
}

func (s *stubImportLabelOwner) AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*model.ImportAccount, error) {
	return s.account, nil
}

func (s *stubImportLabelOwner) CanAddTransaction(ctx context.Context, userID vo.Id, accountID vo.Id) (bool, error) {
	return true, nil
}

func (s *stubImportLabelOwner) CreateAccount(ctx context.Context, userID vo.Id, name string) (model.ImportAccount, error) {
	return model.ImportAccount{}, nil
}

func (s *stubImportLabelOwner) CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return nil, nil
}

func (s *stubImportLabelOwner) PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return nil, nil
}

func (s *stubImportLabelOwner) TagsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	return nil, nil
}

func (s *stubImportLabelOwner) LabelsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	s.labelsByOwnerCalls = append(s.labelsByOwnerCalls, ownerID)
	return s.labelsByOwner[ownerID.String()], nil
}

func (s *stubImportLabelOwner) CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (model.ImportNamed, error) {
	return model.ImportNamed{}, nil
}

func (s *stubImportLabelOwner) CreatePayee(ctx context.Context, ownerID vo.Id, name string) (model.ImportNamed, error) {
	return model.ImportNamed{}, nil
}

func (s *stubImportLabelOwner) CreateTag(ctx context.Context, ownerID vo.Id, name string) (model.ImportNamed, error) {
	return model.ImportNamed{}, nil
}

func (s *stubImportLabelOwner) CreateLabel(ctx context.Context, ownerID vo.Id, name string) (vo.Id, error) {
	s.createLabelCalls = append(s.createLabelCalls, struct {
		ownerID vo.Id
		name    string
	}{ownerID, name})
	if err, ok := s.createLabelErrFor[name]; ok {
		return vo.Id{}, err
	}
	return s.nextLabelID, nil
}

func (s *stubImportLabelOwner) SaveTransaction(ctx context.Context, t *model.Transaction) error {
	return nil
}

// stubImportRepo is a minimal Repository: only NextIdentity/ReplaceLabels are
// exercised by the import path under test.
type stubImportRepo struct {
	replaceLabelsCalls []vo.Id
}

func (r *stubImportRepo) NextIdentity() vo.Id { return vo.NewId() }
func (r *stubImportRepo) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error) {
	return nil, nil
}
func (r *stubImportRepo) Save(ctx context.Context, t *model.Transaction) error { return nil }
func (r *stubImportRepo) LinkRecurring(ctx context.Context, id, recurringID vo.Id, now time.Time) error {
	return nil
}
func (r *stubImportRepo) Delete(ctx context.Context, id vo.Id) error { return nil }
func (r *stubImportRepo) ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.Transaction, error) {
	return nil, nil
}
func (r *stubImportRepo) ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, filter model.TransactionFilter) ([]*model.Transaction, error) {
	return nil, nil
}
func (r *stubImportRepo) ReplaceLabels(ctx context.Context, transactionID vo.Id, labelIDs []vo.Id) error {
	r.replaceLabelsCalls = append(r.replaceLabelsCalls, transactionID)
	return nil
}
func (r *stubImportRepo) LabelsByTransactionIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error) {
	return nil, nil
}

type passthroughTx struct{}

func (passthroughTx) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// TestImportRow_LabelAutoCreate_ScopesToAccountOwnerNotCaller pins
// internal/transaction/import.go's auto-create call (imp.CreateLabel(ctx,
// accountOwnerID, name)): a co-sharer (userID) importing into an account
// owned by someone else (ownerID, via the accountId override) must have the
// missing mapped label created under the OWNER, never the caller — else the
// owner can never resolve the label by name on their own books. Mutating
// that call's second argument from accountOwnerID to userID makes this test
// fail (verified below).
func TestImportRow_LabelAutoCreate_ScopesToAccountOwnerNotCaller(t *testing.T) {
	userID := vo.NewId()
	ownerID := vo.NewId()
	acctID := vo.NewId()

	imp := &stubImportLabelOwner{
		account:       &model.ImportAccount{ID: acctID.String(), Name: "Shared", OwnerID: ownerID.String()},
		labelsByOwner: map[string][]model.ImportNamed{}, // no existing labels anywhere -> forces auto-create
		nextLabelID:   vo.NewId(),
	}
	svc := &Service{importer: imp, repo: &stubImportRepo{}, tx: passthroughTx{}, clock: fixedClock{now: time.Now()}}

	acctIDStr := acctID.String()
	dateStr := "2024-03-01"
	req := model.ImportRequest{
		File:      []byte("Amount,Labels\n-10,New Label\n"),
		Mapping:   model.ImportMapping{Amount: "Amount", Labels: "Labels"},
		AccountId: &acctIDStr,
		Date:      &dateStr,
	}

	res, err := svc.ImportTransactionList(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("ImportTransactionList: %v", err)
	}
	if res.Imported != 1 || res.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d want 1/0; errors=%v", res.Imported, res.Skipped, res.Errors)
	}
	if len(imp.createLabelCalls) != 1 {
		t.Fatalf("CreateLabel calls = %d, want 1", len(imp.createLabelCalls))
	}
	if got := imp.createLabelCalls[0].ownerID; !got.Equal(ownerID) {
		t.Fatalf("CreateLabel owner = %s, want the account owner %s (not the caller %s)", got, ownerID, userID)
	}
}

// TestImportOverride_LabelIds_BelongsToCheckScopesToAccountOwnerNotCaller
// pins the labelIds override's authorization list: it must come from
// imp.LabelsByOwner(ctx, accountOwnerID), not the caller's own labels. The
// stub's LabelsByOwner returns a DIFFERENT set per owner id (the caller has
// one label, the account owner has none), so a labelIds override naming the
// caller's own label must be rejected as not-found when importing into the
// other user's account. Mutating the LabelsByOwner call site's argument from
// accountOwnerID to userID makes this test fail (verified below).
func TestImportOverride_LabelIds_BelongsToCheckScopesToAccountOwnerNotCaller(t *testing.T) {
	userID := vo.NewId()
	ownerID := vo.NewId()
	acctID := vo.NewId()
	callerLabelID := vo.NewId()

	imp := &stubImportLabelOwner{
		account: &model.ImportAccount{ID: acctID.String(), Name: "Shared", OwnerID: ownerID.String()},
		labelsByOwner: map[string][]model.ImportNamed{
			userID.String():  {{ID: callerLabelID.String(), Name: "Caller Label", OwnerID: userID.String()}},
			ownerID.String(): {}, // the account owner has no labels
		},
	}
	svc := &Service{importer: imp, repo: &stubImportRepo{}, tx: passthroughTx{}, clock: fixedClock{now: time.Now()}}

	acctIDStr := acctID.String()
	dateStr := "2024-03-01"
	labelIDsStr := callerLabelID.String()
	req := model.ImportRequest{
		File:      []byte("Amount\n-10\n"),
		Mapping:   model.ImportMapping{Amount: "Amount"},
		AccountId: &acctIDStr,
		Date:      &dateStr,
		LabelIds:  &labelIDsStr,
	}

	res, err := svc.ImportTransactionList(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("ImportTransactionList: %v", err)
	}
	if res.Imported != 0 {
		t.Fatalf("imported=%d want 0 (the caller's own label does not belong to the account owner)", res.Imported)
	}
	if _, ok := res.Errors["Label not found for provided labelIds"]; !ok {
		t.Fatalf("errors=%v want top-level label-not-found error", res.Errors)
	}
	if len(imp.labelsByOwnerCalls) == 0 || !imp.labelsByOwnerCalls[0].Equal(ownerID) {
		t.Fatalf("LabelsByOwner calls = %v, want first call scoped to the account owner %s", imp.labelsByOwnerCalls, ownerID)
	}
}
