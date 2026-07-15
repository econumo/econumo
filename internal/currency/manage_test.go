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
	rates   []model.RateRow
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

func (f *fakeManageRepo) UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int) error {
	rec, ok := f.records[id]
	if !ok {
		return errs.NewNotFound("Currency not found")
	}
	rec.Name = &name
	rec.Symbol = symbol
	rec.FractionDigits = fractionDigits
	f.records[id] = rec
	return nil
}

func (f *fakeManageRepo) SetCurrencyArchived(ctx context.Context, id string, archived bool) error {
	rec, ok := f.records[id]
	if !ok {
		return errs.NewNotFound("Currency not found")
	}
	rec.IsArchived = archived
	f.records[id] = rec
	return nil
}

func (f *fakeManageRepo) DeleteCurrency(ctx context.Context, id string) error {
	delete(f.records, id)
	return nil
}

func (f *fakeManageRepo) CountCurrencyUsage(ctx context.Context, id, code string) (int64, error) {
	return f.usage[id], nil
}

func (f *fakeManageRepo) GetGlobalIDByCode(ctx context.Context, code string) (string, error) {
	for _, r := range f.records {
		if r.UserID == nil && r.Code == code {
			return r.ID, nil
		}
	}
	return "", errs.NewNotFound("Currency not found")
}

func (f *fakeManageRepo) UpsertRate(ctx context.Context, r model.RateRow) error {
	f.rates = append(f.rates, r)
	return nil
}

func (f *fakeManageRepo) HideCurrency(ctx context.Context, userID, currencyID string, now time.Time) error {
	f.hidden[userID+"|"+currencyID] = true
	return nil
}

func (f *fakeManageRepo) ShowCurrency(ctx context.Context, userID, currencyID string) error {
	delete(f.hidden, userID+"|"+currencyID)
	return nil
}

var _ appcurrency.ManageModel = (*fakeManageRepo)(nil)

type fakeProfileCurrency struct{ code string }

func (f fakeProfileCurrency) CurrencyCode(ctx context.Context, userID string) (string, error) {
	return f.code, nil
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
	return appcurrency.NewManageService(repo, passthroughTx{}, ops, fixedClock{t: now}, fakeProfileCurrency{code: "USD"}, "USD")
}

var manageNow = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

func TestCreateCurrency_HappyPath(t *testing.T) {
	repo := newFakeManageRepo()
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points"}

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
}

func TestCreateCurrency_WithInitialRate(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$", FractionDigits: 2}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Rate: strPtr("1.5")}

	res, err := svc.CreateCurrency(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateCurrency: %v", err)
	}
	if len(repo.rates) != 1 {
		t.Fatalf("rates upserted = %d, want 1", len(repo.rates))
	}
	rr := repo.rates[0]
	if rr.CurrencyID != res.Item.Id {
		t.Errorf("rate CurrencyID = %q, want %q", rr.CurrencyID, res.Item.Id)
	}
	if rr.BaseCurrencyID != "global-usd" {
		t.Errorf("rate BaseCurrencyID = %q, want global-usd", rr.BaseCurrencyID)
	}
	if rr.Rate != "1.5" {
		t.Errorf("rate Rate = %q, want 1.5", rr.Rate)
	}
	wantDate := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if !rr.Date.Equal(wantDate) {
		t.Errorf("rate Date = %v, want %v", rr.Date, wantDate)
	}
}

func TestCreateCurrency_BadCode(t *testing.T) {
	for _, code := range []string{"pt", "P!S"} {
		t.Run(code, func(t *testing.T) {
			repo := newFakeManageRepo()
			svc := newManageSvc(repo, &fakeOps{}, manageNow)
			uid := vo.MustParseId(manageMeID)
			req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: code, Name: "X"}

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
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points"}

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
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "usd", Name: "US Dollar"}

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
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: strings.Repeat("a", 65)}

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
	req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", Symbol: strPtr(strings.Repeat("a", 13))}

	_, err := svc.CreateCurrency(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "symbol", "Currency symbol must be 1-12 characters")
}

func TestCreateCurrency_FractionDigitsRange(t *testing.T) {
	for _, d := range []int{-1, 9} {
		repo := newFakeManageRepo()
		svc := newManageSvc(repo, &fakeOps{}, manageNow)
		uid := vo.MustParseId(manageMeID)
		req := model.CreateCurrencyRequest{Id: vo.NewId().String(), Code: "pts", Name: "Points", FractionDigits: intPtr(d)}

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
	req := model.CreateCurrencyRequest{Id: opID, Code: "pts", Name: "Points"}

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
	req := model.UpdateCustomCurrencyRequest{Id: "own-pts", Name: "Reward Points", Symbol: "RP", FractionDigits: 0}

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
			req := model.UpdateCustomCurrencyRequest{Id: rec.ID, Name: "X", Symbol: "X", FractionDigits: 2}

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
	req := model.UpdateCustomCurrencyRequest{Id: "missing", Name: "X", Symbol: "X", FractionDigits: 2}

	_, err := svc.UpdateCurrency(context.Background(), uid, req)
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("err = %v, want *NotFoundError", err)
	}
}

func TestArchiveCurrency_OwnerOnly(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	repo.records["foreign-pts"] = model.CurrencyRecord{ID: "foreign-pts", Code: "GEM", Symbol: "GEM", UserID: strPtr(manageOtherID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.ArchiveCurrency(context.Background(), uid, model.ArchiveCurrencyRequest{Id: "own-pts"}); err != nil {
		t.Fatalf("ArchiveCurrency (owner): %v", err)
	}
	if !repo.records["own-pts"].IsArchived {
		t.Error("expected own-pts to be archived")
	}

	_, err := svc.ArchiveCurrency(context.Background(), uid, model.ArchiveCurrencyRequest{Id: "foreign-pts"})
	if _, ok := errs.AsAccessDenied(err); !ok {
		t.Fatalf("err = %v, want *AccessDeniedError", err)
	}
}

func TestUnarchiveCurrency(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID), IsArchived: true}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)

	if _, err := svc.UnarchiveCurrency(context.Background(), uid, model.UnarchiveCurrencyRequest{Id: "own-pts"}); err != nil {
		t.Fatalf("UnarchiveCurrency: %v", err)
	}
	if repo.records["own-pts"].IsArchived {
		t.Error("expected own-pts to be unarchived")
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
	if _, ok := repo.records["own-pts"]; ok {
		t.Error("record should have been deleted")
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

func TestSetCurrencyRate_HappyPathDefaultDate(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$"}
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.SetCurrencyRateRequest{CurrencyId: "own-pts", Rate: "1.5"}

	if _, err := svc.SetCurrencyRate(context.Background(), uid, req); err != nil {
		t.Fatalf("SetCurrencyRate: %v", err)
	}
	if len(repo.rates) != 1 {
		t.Fatalf("rates upserted = %d, want 1", len(repo.rates))
	}
	rr := repo.rates[0]
	if rr.CurrencyID != "own-pts" || rr.BaseCurrencyID != "global-usd" || rr.Rate != "1.5" {
		t.Errorf("rate = %+v, want CurrencyID=own-pts BaseCurrencyID=global-usd Rate=1.5", rr)
	}
	wantDate := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if !rr.Date.Equal(wantDate) {
		t.Errorf("rate Date = %v, want %v", rr.Date, wantDate)
	}
}

func TestSetCurrencyRate_ExplicitDate(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$"}
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.SetCurrencyRateRequest{CurrencyId: "own-pts", Rate: "1.5", Date: strPtr("2026-01-15")}

	if _, err := svc.SetCurrencyRate(context.Background(), uid, req); err != nil {
		t.Fatalf("SetCurrencyRate: %v", err)
	}
	wantDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !repo.rates[0].Date.Equal(wantDate) {
		t.Errorf("rate Date = %v, want %v", repo.rates[0].Date, wantDate)
	}
}

func TestSetCurrencyRate_BadDate(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$"}
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.SetCurrencyRateRequest{CurrencyId: "own-pts", Rate: "1.5", Date: strPtr("15/01/2026")}

	_, err := svc.SetCurrencyRate(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "date", "Date is not valid")
}

func TestSetCurrencyRate_BadRate(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.SetCurrencyRateRequest{CurrencyId: "own-pts", Rate: "0"}

	_, err := svc.SetCurrencyRate(context.Background(), uid, req)
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	assertField(t, v, "rate", "Rate must be a positive number")
}

func TestSetCurrencyRate_GlobalTarget(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-usd"] = model.CurrencyRecord{ID: "global-usd", Code: "USD", Symbol: "$"}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.SetCurrencyRateRequest{CurrencyId: "global-usd", Rate: "1.5"}

	_, err := svc.SetCurrencyRate(context.Background(), uid, req)
	ad, ok := errs.AsAccessDenied(err)
	if !ok {
		t.Fatalf("err = %v, want *AccessDeniedError", err)
	}
	if ad.Msg != "" {
		t.Errorf("Msg = %q, want empty", ad.Msg)
	}
}

func TestSetCurrencyRate_ForeignTarget(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["foreign-pts"] = model.CurrencyRecord{ID: "foreign-pts", Code: "GEM", Symbol: "GEM", UserID: strPtr(manageOtherID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
	uid := vo.MustParseId(manageMeID)
	req := model.SetCurrencyRateRequest{CurrencyId: "foreign-pts", Rate: "1.5"}

	_, err := svc.SetCurrencyRate(context.Background(), uid, req)
	ad, ok := errs.AsAccessDenied(err)
	if !ok {
		t.Fatalf("err = %v, want *AccessDeniedError", err)
	}
	if ad.Msg != "" {
		t.Errorf("Msg = %q, want empty", ad.Msg)
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

func TestHideCurrency_CustomTarget(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["own-pts"] = model.CurrencyRecord{ID: "own-pts", Code: "PTS", Symbol: "PTS", UserID: strPtr(manageMeID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)
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
	svc := appcurrency.NewManageService(repo, passthroughTx{}, &fakeOps{}, fixedClock{t: manageNow}, fakeProfileCurrency{code: "EUR"}, "USD")
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
