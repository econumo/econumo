package currency_test

// Service-level tests for ManageService's lifecycle use cases (create, update,
// archive, unarchive, delete), against in-package fakes (no DB). The DB-backed
// ManageRepo itself is covered by internal/currency/repo/manage_integration_test.go.

import (
	"context"
	"strings"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	manageMeID    = "10000000-0000-7000-8000-000000000011"
	manageOtherID = "20000000-0000-7000-8000-000000000022"
)

func intPtr(i int) *int { return &i }

type fakeManageRepo struct {
	records map[string]model.CurrencyRecord
	usage   map[string]int64
	hidden  map[string]bool
}

func newFakeManageRepo() *fakeManageRepo {
	return &fakeManageRepo{
		records: map[string]model.CurrencyRecord{},
		usage:   map[string]int64{},
		hidden:  map[string]bool{},
	}
}

func (f *fakeManageRepo) GetCurrencyRecord(ctx context.Context, id string) (model.CurrencyRecord, error) {
	rec, ok := f.records[id]
	if !ok {
		return model.CurrencyRecord{}, errs.NewNotFound("Currency not found")
	}
	return rec, nil
}

func (f *fakeManageRepo) GlobalCodeExists(ctx context.Context, code string) (bool, error) {
	for _, r := range f.records {
		if r.UserID == nil && r.Code == code {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeManageRepo) OwnerCodeExists(ctx context.Context, userID, code string) (bool, error) {
	for _, r := range f.records {
		if r.UserID != nil && *r.UserID == userID && r.Code == code {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeManageRepo) InsertUserCurrency(ctx context.Context, c model.CurrencyRecord) error {
	f.records[c.ID] = c
	return nil
}

func (f *fakeManageRepo) UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int, rate *string) error {
	rec, ok := f.records[id]
	if !ok {
		return errs.NewNotFound("Currency not found")
	}
	rec.Name = &name
	rec.Symbol = symbol
	rec.FractionDigits = fractionDigits
	rec.Rate = rate
	f.records[id] = rec
	return nil
}

func (f *fakeManageRepo) SoftDeleteCurrency(ctx context.Context, id string) error {
	rec, ok := f.records[id]
	if !ok {
		return errs.NewNotFound("Currency not found")
	}
	rec.IsDeleted = true
	f.records[id] = rec
	return nil
}

func (f *fakeManageRepo) CountCurrencyUsage(ctx context.Context, id string) (int64, error) {
	return f.usage[id], nil
}

func (f *fakeManageRepo) HideCurrency(ctx context.Context, userID, currencyID string, now time.Time) error {
	f.hidden[userID+"|"+currencyID] = true
	return nil
}

func (f *fakeManageRepo) ShowCurrency(ctx context.Context, userID, currencyID string) error {
	delete(f.hidden, userID+"|"+currencyID)
	return nil
}

func (f *fakeManageRepo) HideGlobalCurrencies(ctx context.Context, userID string, excludeIDs []string, now time.Time) error {
	excluded := map[string]bool{}
	for _, id := range excludeIDs {
		excluded[id] = true
	}
	for _, r := range f.records {
		if r.UserID == nil && !excluded[r.ID] {
			f.hidden[userID+"|"+r.ID] = true
		}
	}
	return nil
}

func (f *fakeManageRepo) ShowGlobalCurrencies(ctx context.Context, userID string) error {
	for _, r := range f.records {
		if r.UserID == nil {
			delete(f.hidden, userID+"|"+r.ID)
		}
	}
	return nil
}

var _ appcurrency.ManageModel = (*fakeManageRepo)(nil)

type fakeProfileCurrency struct{ id string }

func (f fakeProfileCurrency) CurrencyID(ctx context.Context, userID string) (string, error) {
	return f.id, nil
}

var _ appcurrency.ProfileCurrency = fakeProfileCurrency{}

type passthroughTx struct{}

func (passthroughTx) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

type fakeOps struct {
	claimed map[string]bool
}

func (f *fakeOps) Claim(ctx context.Context, id vo.Id, now time.Time) (bool, error) {
	if f.claimed == nil {
		f.claimed = map[string]bool{}
	}
	k := id.String()
	if f.claimed[k] {
		return true, nil
	}
	f.claimed[k] = true
	return false, nil
}

func (f *fakeOps) MarkHandled(ctx context.Context, id vo.Id, now time.Time) error { return nil }

func newManageSvc(repo *fakeManageRepo, ops *fakeOps, now time.Time) *appcurrency.ManageService {
	return newManageSvcWithProfile(repo, ops, now, &fakeProfileCurrency{id: "global-usd"})
}

func newManageSvcWithProfile(repo *fakeManageRepo, ops *fakeOps, now time.Time, profile *fakeProfileCurrency) *appcurrency.ManageService {
	return appcurrency.NewManageService(repo, passthroughTx{}, ops, fixedClock{t: now}, *profile, testBase)
}

var manageNow = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

// testBase mirrors the boot-resolved base: the fakes register the global USD
// record under the id "global-usd".
var testBase = model.BaseCurrency{ID: "global-usd", Code: "USD"}

func TestCreateCurrency_HappyPath(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Rate: strPtr("1")}

	res, err := svc.CreateCurrency(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateCurrency: %v", err)
	}
	if res.Item.Code != "PTS" {
		t.Errorf("Code = %q, want PTS", res.Item.Code)
	}
	if res.Item.Symbol != "PTS" {
		t.Errorf("Symbol = %q, want PTS (defaults to code)", res.Item.Symbol)
	}
	if res.Item.FractionDigits != 2 {
		t.Errorf("FractionDigits = %d, want 2", res.Item.FractionDigits)
	}
	if res.Item.Scope != appcurrency.ScopeOwn {
		t.Errorf("Scope = %q, want own", res.Item.Scope)
	}
	rec, ok := repo.records[res.Item.Id]
	if !ok {
		t.Fatal("expected the currency to be persisted")
	}
	if rec.UserID == nil || *rec.UserID != manageMeID {
		t.Errorf("persisted UserID = %v, want %s", rec.UserID, manageMeID)
	}
	if rec.Rate == nil || *rec.Rate != "1" {
		t.Errorf("persisted Rate = %v, want the fixed rate 1", rec.Rate)
	}
}

// The fixed rate lives ON the currency record: no rate rows are ever written
// for customs.
func TestCreateCurrency_StoresFixedRate(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Rate: strPtr("1.5")}

	res, err := svc.CreateCurrency(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateCurrency: %v", err)
	}
	rec := repo.records[res.Item.Id]
	if rec.Rate == nil || *rec.Rate != "1.5" {
		t.Fatalf("persisted Rate = %v, want 1.5", rec.Rate)
	}
}

// The rate is mandatory: a custom currency without one cannot exist.
func TestCreateCurrency_MissingRate(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points"}

	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "rate", "This value should not be blank.")
}

func TestCreateCurrency_BadCode(t *testing.T) {
	for _, code := range []string{"pt", "P!S"} {
		t.Run(code, func(t *testing.T) {
			repo := newFakeManageRepo()
			svc := newManageSvc(repo, &fakeOps{}, manageNow)
			uid := vo.MustParseId(manageMeID)
			req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: code, Name: "X", Rate: strPtr("1")}

			_, err := svc.CreateCurrency(context.Background(), uid, req)
			v, ok := errs.AsValidation(err)
			if !ok {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
			assertField(t, v, "code", "CurrencyCode is incorrect")
		})
	}
}

func TestCreateCurrency_DuplicateOwnCode(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Rate: strPtr("1")}

	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "code", "Currency already exists")
}

func TestCreateCurrency_CollidesWithGlobalCode(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$"}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "usd", Name: "US Dollar", Rate: strPtr("1")}

	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "code", "Currency already exists")
}

func TestCreateCurrency_NameLength(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: strings.Repeat("a", 65), Rate: strPtr("1")}

	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "name", "Currency name must be 1-64 characters")
}

func TestCreateCurrency_SymbolLength(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Symbol: strPtr(strings.Repeat("a", 13)), Rate: strPtr("1")}

	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "symbol", "Currency symbol must be 1-12 characters")
}

func TestCreateCurrency_NameLength_MultibyteWithinLimit(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	name := strings.Repeat("я", 33)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: name, Rate: strPtr("1")}

	res, err := svc.CreateCurrency(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateCurrency: %v", err)
	}
	if res.Item.Name != name {
		t.Errorf("Name = %q, want %q", res.Item.Name, name)
	}
}

func TestCreateCurrency_SymbolLength_MultibyteWithinLimit(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	symbol := "₽₽₽₽₽"
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Symbol: strPtr(symbol), Rate: strPtr("1")}

	res, err := svc.CreateCurrency(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateCurrency: %v", err)
	}
	if res.Item.Symbol != symbol {
		t.Errorf("Symbol = %q, want %q", res.Item.Symbol, symbol)
	}
}

func TestCreateCurrency_FractionDigitsRange(t *testing.T) {
	for _, d := range []int{-1, 9} {
		repo := newFakeManageRepo()
		svc := newManageSvc(repo, &fakeOps{}, manageNow)
		uid := vo.MustParseId(manageMeID)
		req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", FractionDigits: intPtr(d), Rate: strPtr("1")}

		_, err := svc.CreateCurrency(context.Background(), uid, req)
		v, ok := errs.AsValidation(err)
		if !ok {
			t.Fatalf("digits=%d: err = %v, want *ValidationError", d, err)
		}
		assertField(t, v, "fractionDigits", "Fraction digits must be between 0 and 8")
	}
}

func TestCreateCurrency_BadRate(t *testing.T) {
	for _, rate := range []string{"0", "-1", "abc"} {
		t.Run(rate, func(t *testing.T) {
			repo := newFakeManageRepo()
			svc := newManageSvc(repo, &fakeOps{}, manageNow)
			uid := vo.MustParseId(manageMeID)
			req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Rate: strPtr(rate)}

			_, err := svc.CreateCurrency(context.Background(), uid, req)
			v, ok := errs.AsValidation(err)
			if !ok {
				t.Fatalf("rate=%q: err = %v, want *ValidationError", rate, err)
			}
			assertField(t, v, "rate", "Rate must be a positive number")
		})
	}
}

func TestCreateCurrency_DuplicateOperation(t *testing.T) {
	repo := newFakeManageRepo()
	ops := &fakeOps{}
	svc := newManageSvc(repo, ops, manageNow)
	uid := vo.MustParseId(manageMeID)
	opID := vo.NewId().String()
	req := model.CreateCurrencyRequest{Id: opID, Code: "pts", Name: "Points", Rate: strPtr("1")}

	if _, err := svc.CreateCurrency(context.Background(), uid, req); err != nil {
		t.Fatalf("first CreateCurrency: %v", err)
	}
	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if v.Msg != "Operation is locked" {
		t.Errorf("Msg = %q, want %q", v.Msg, "Operation is locked")
	}
}

func TestUpdateCurrency_HappyPath(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", FractionDigits: 2, UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.UpdateCustomCurrencyRequest{Id: "own-pts", Name: "Reward Points", Symbol: "RP", FractionDigits: 0, Rate: "2.5"}

	res, err := svc.UpdateCurrency(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("UpdateCurrency: %v", err)
	}
	if res.Item.Name != "Reward Points" || res.Item.Symbol != "RP" || res.Item.FractionDigits != 0 {
		t.Errorf("Item = %+v, want updated fields", res.Item)
	}
	rec := repo.records["own-pts"]
	if rec.Symbol != "RP" || rec.FractionDigits != 0 {
		t.Errorf("persisted record = %+v, want updated fields", rec)
	}
	if rec.Rate == nil || *rec.Rate != "2.5" {
		t.Errorf("persisted Rate = %v, want 2.5 (update re-values history)", rec.Rate)
	}
}

// Update is a full replace and the rate is part of it — omitting it is a
// validation error, never a clear.
func TestUpdateCurrency_MissingRate(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", FractionDigits: 2, UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.UpdateCustomCurrencyRequest{Id: "own-pts", Name: "Reward Points", Symbol: "RP", FractionDigits: 0}

	_, err := svc.UpdateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "rate", "This value should not be blank.")
}

func TestUpdateCurrency_NotOwner(t *testing.T) {
	cases := map[string]model.CurrencyRecord{
		"global":  {ID: "global-usd", Code: "USD", Symbol: "$"},
		"foreign": {ID: "foreign-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageOtherID)},
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newFakeManageRepo()
			repo.records[rec.ID] = rec
			svc := newManageSvc(repo, &fakeOps{}, manageNow)
			uid := vo.MustParseId(manageMeID)
			req := model.UpdateCustomCurrencyRequest{Id: rec.ID, Name: "X", Symbol: "X", FractionDigits: 2, Rate: "2.5"}

			_, err := svc.UpdateCurrency(context.Background(), uid, req)
			ad, ok := errs.AsAccessDenied(err)
			if !ok {
				t.Fatalf("err = %v, want *AccessDeniedError", err)
			}
			if ad.Msg != "" {
				t.Errorf("Msg = %q, want empty", ad.Msg)
			}
		})
	}
}

func TestUpdateCurrency_NotFound(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.UpdateCustomCurrencyRequest{Id: "missing", Name: "X", Symbol: "X", FractionDigits: 2, Rate: "2.5"}

	_, err := svc.UpdateCurrency(context.Background(), uid, req)
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("err = %v, want *NotFoundError", err)
	}
}

func TestDeleteCurrency_RefusesWhenUsed(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	repo.usage["own-pts"] = 1
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	_, err := svc.DeleteCurrency(context.Background(), uid, model.DeleteCurrencyRequest{Id: "own-pts"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if v.Msg != "Currency is in use and cannot be deleted" {
		t.Errorf("Msg = %q, want %q", v.Msg, "Currency is in use and cannot be deleted")
	}
	if _, ok := repo.records["own-pts"]; !ok {
		t.Error("record should NOT have been deleted")
	}
}

func TestDeleteCurrency_HappyPath(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.DeleteCurrency(context.Background(), uid, model.DeleteCurrencyRequest{Id: "own-pts"}); err != nil {
		t.Fatalf("DeleteCurrency: %v", err)
	}
	rec, ok := repo.records["own-pts"]
	if !ok {
		t.Fatal("the record must survive a delete (currencies are never hard-deleted)")
	}
	if !rec.IsDeleted {
		t.Error("record should have IsDeleted = true")
	}
}

func TestDeleteCurrency_NotOwner(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["foreign-pts"] = model.CurrencyRecord{ID: "foreign-pts", Code: "GEM", Symbol: "GEM", UserID: strPtr(manageOtherID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	_, err := svc.DeleteCurrency(context.Background(), uid, model.DeleteCurrencyRequest{Id: "foreign-pts"})
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("err = %v, want *AccessDeniedError", err)
	}
}

func TestHideCurrency_HappyPath(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-gbp"] = model.CurrencyRecord{ID: "global-gbp", Code: "GBP", Symbol: "£"}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.HideCurrency(context.Background(), uid, model.HideCurrencyRequest{Id: "global-gbp"}); err != nil {
		t.Fatalf("HideCurrency: %v", err)
	}
	if !repo.hidden[manageMeID+"|global-gbp"] {
		t.Error("expected global-gbp to be hidden for the caller")
	}
}

// Hiding your OWN custom is allowed: it replaces archival as the way to
// retire a currency from your pickers.
func TestHideCurrency_OwnCustomAllowed(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.HideCurrency(context.Background(), uid, model.HideCurrencyRequest{Id: "own-pts"}); err != nil {
		t.Fatalf("HideCurrency(own custom): %v", err)
	}
	if !repo.hidden[manageMeID+"|own-pts"] {
		t.Fatal("expected the own custom to be hidden")
	}
}

// A FOREIGN custom stays refused.
func TestHideCurrency_ForeignCustomRefused(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["their-pts"] = model.CurrencyRecord{ID: "their-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageOtherID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	_, err := svc.HideCurrency(context.Background(), uid, model.HideCurrencyRequest{Id: "their-pts"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if v.Msg != "This currency cannot be hidden" {
		t.Errorf("Msg = %q, want %q", v.Msg, "This currency cannot be hidden")
	}
}

// Hiding your own custom while it is your profile default stays refused
// (the existing profile guard, now the only guard).
func TestHideCurrency_OwnDefaultRefused(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := appcurrency.NewManageService(repo, passthroughTx{}, &fakeOps{}, fixedClock{t: manageNow}, fakeProfileCurrency{id: "own-pts"}, testBase)
	uid := vo.MustParseId(manageMeID)

	_, err := svc.HideCurrency(context.Background(), uid, model.HideCurrencyRequest{Id: "own-pts"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if v.Msg != "This currency cannot be hidden" {
		t.Errorf("Msg = %q, want %q", v.Msg, "This currency cannot be hidden")
	}
}

func TestHideCurrency_BaseCurrency(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$"}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	_, err := svc.HideCurrency(context.Background(), uid, model.HideCurrencyRequest{Id: "global-usd"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if v.Msg != "The base currency cannot be modified" {
		t.Errorf("Msg = %q, want %q", v.Msg, "The base currency cannot be modified")
	}
}

func TestHideCurrency_ProfileCurrency(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-eur"] = model.CurrencyRecord{ID: "global-eur", Code: "EUR", Symbol: "€"}
	svc := appcurrency.NewManageService(repo, passthroughTx{}, &fakeOps{}, fixedClock{t: manageNow}, fakeProfileCurrency{id: "global-eur"}, testBase)
	uid := vo.MustParseId(manageMeID)

	_, err := svc.HideCurrency(context.Background(), uid, model.HideCurrencyRequest{Id: "global-eur"})
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if v.Msg != "This currency cannot be hidden" {
		t.Errorf("Msg = %q, want %q", v.Msg, "This currency cannot be hidden")
	}
}

func TestShowCurrency_HappyPath(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-gbp"] = model.CurrencyRecord{ID: "global-gbp", Code: "GBP", Symbol: "£"}
	repo.hidden[manageMeID+"|global-gbp"] = true
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.ShowCurrency(context.Background(), uid, model.ShowCurrencyRequest{Id: "global-gbp"}); err != nil {
		t.Fatalf("ShowCurrency: %v", err)
	}
	if repo.hidden[manageMeID+"|global-gbp"] {
		t.Error("expected global-gbp to no longer be hidden")
	}

	if _, err := svc.ShowCurrency(context.Background(), uid, model.ShowCurrencyRequest{Id: "global-gbp"}); err != nil {
		t.Fatalf("ShowCurrency (idempotent): %v", err)
	}
	if repo.hidden[manageMeID+"|global-gbp"] {
		t.Error("expected global-gbp to remain shown")
	}
}

func assertField(t *testing.T, v *errs.ValidationError, key, message string) {
	t.Helper()
	for _, f := range v.Fields {
		if f.Key == key && f.Message == message {
			return
		}
	}
	t.Errorf("Fields = %+v, want a field {Key:%q Message:%q}", v.Fields, key, message)
}

// Bulk disable: every global except the base and the caller's profile default
// becomes hidden; customs (own and foreign) are untouched.
func TestHideAllCurrencies_SkipsBaseProfileAndCustoms(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD"}
	repo.records["global-eur"] = model.CurrencyRecord{ID: "global-eur", Code: "EUR"}
	repo.records["global-cad"] = model.CurrencyRecord{ID: "global-cad", Code: "CAD"}
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvcWithProfile(repo, &fakeOps{}, manageNow, &fakeProfileCurrency{id: "global-eur"})
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.HideAllCurrencies(context.Background(), uid); err != nil {
		t.Fatalf("HideAllCurrencies: %v", err)
	}
	if repo.hidden[manageMeID+"|global-usd"] {
		t.Error("base currency must not be hidden")
	}
	if repo.hidden[manageMeID+"|global-eur"] {
		t.Error("profile currency must not be hidden")
	}
	if !repo.hidden[manageMeID+"|global-cad"] {
		t.Error("expected the remaining global to be hidden")
	}
	if repo.hidden[manageMeID+"|own-pts"] {
		t.Error("own customs are out of bulk scope")
	}
}

// Bulk enable: every hidden global is shown again; a hidden own custom stays
// hidden (out of bulk scope).
func TestShowAllCurrencies_GlobalsOnly(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-cad"] = model.CurrencyRecord{ID: "global-cad", Code: "CAD"}
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", UserID: strPtr(manageMeID)}
	repo.hidden[manageMeID+"|global-cad"] = true
	repo.hidden[manageMeID+"|own-pts"] = true
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.ShowAllCurrencies(context.Background(), uid); err != nil {
		t.Fatalf("ShowAllCurrencies: %v", err)
	}
	if repo.hidden[manageMeID+"|global-cad"] {
		t.Error("expected the hidden global to be shown again")
	}
	if !repo.hidden[manageMeID+"|own-pts"] {
		t.Error("a hidden own custom stays hidden")
	}
}
