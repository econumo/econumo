package budget_test

import (
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (h *debtHarness) foreignCustomCurrency(t *testing.T) string {
	t.Helper()
	other := h.f.User(fixture.User{Name: "Other", Email: "other@envelope-currency.test"})
	return h.f.Currency(fixture.Currency{Code: "ZZF", Symbol: "F", UserID: other, Rate: "2.00000000"})
}

func (h *debtHarness) ownCustomCurrency(t *testing.T) string {
	t.Helper()
	return h.f.Currency(fixture.Currency{Code: "ZZO", Symbol: "O", UserID: h.owner.String(), Rate: "3.00000000"})
}

func (h *debtHarness) createEnvelope(currencyID string) (string, error) {
	id := vo.NewId().String()
	_, err := h.svc.CreateEnvelope(h.ctx, h.owner, model.CreateEnvelopeRequest{
		BudgetId: h.budgetID, Id: id, Name: "Housing", Icon: "home", CurrencyId: currencyID,
	})
	return id, err
}

func (h *debtHarness) updateEnvelope(id, name, currencyID string) error {
	_, err := h.svc.UpdateEnvelope(h.ctx, h.owner, model.UpdateEnvelopeRequest{
		BudgetId: h.budgetID, Id: id, Name: name, Icon: "home", CurrencyId: currencyID,
	})
	return err
}

func (h *debtHarness) elementCurrency(t *testing.T, envelopeID string) string {
	t.Helper()
	var c string
	if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind("SELECT currency_id FROM budgets_elements WHERE external_id = ?"), envelopeID).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCreateEnvelope_RefusesAForeignCustomCurrency(t *testing.T) {
	h := newDebtHarness(t)
	foreign := h.foreignCustomCurrency(t)

	id, err := h.createEnvelope(foreign)
	requireFieldCode(t, err, "currencyId", errs.CodeCurrencyNotAvailable)
	if n := h.count(t, "SELECT COUNT(*) FROM budgets_envelopes WHERE id = ?", id); n != 0 {
		t.Errorf("envelope rows = %d, want none", n)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM budgets_elements WHERE external_id = ?", id); n != 0 {
		t.Errorf("element rows = %d, want none", n)
	}

	for name, cur := range map[string]string{"global": h.eurCur, "own custom": h.ownCustomCurrency(t)} {
		if _, err := h.createEnvelope(cur); err != nil {
			t.Errorf("creating an envelope in a %s currency: %v", name, err)
		}
	}
}

func TestUpdateEnvelope_RefusesAForeignCustomCurrency(t *testing.T) {
	h := newDebtHarness(t)
	foreign := h.foreignCustomCurrency(t)
	housing := h.envelope(t, "Housing")

	err := h.updateEnvelope(housing, "Housing", foreign)
	requireFieldCode(t, err, "currencyId", errs.CodeCurrencyNotAvailable)
	if got := h.elementCurrency(t, housing); got != fixture.USD {
		t.Errorf("element currency = %s, want it kept in USD", got)
	}

	for name, cur := range map[string]string{"global": h.eurCur, "own custom": h.ownCustomCurrency(t)} {
		if err := h.updateEnvelope(housing, "Housing", cur); err != nil {
			t.Errorf("moving the envelope to a %s currency: %v", name, err)
		}
	}

	// a state the API no longer reaches: the envelope already carries a
	// currency the caller could not pick today
	if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind("UPDATE budgets_elements SET currency_id = ? WHERE external_id = ?"), foreign, housing); err != nil {
		t.Fatal(err)
	}
	if err := h.updateEnvelope(housing, "Home", foreign); err != nil {
		t.Fatalf("renaming an envelope that keeps its currency: %v", err)
	}
}
