package simplefin_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/imports/simplefin"
	"github.com/econumo/econumo/internal/model"
)

func TestParse(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	raw := json.RawMessage(`{"id":"TRN-1","posted":1755900000,"amount":"-12.50","description":"COFFEE SHOP","payee":"Blue Bottle"}`)
	payload := imports.EncodePullEvent(model.ExternalTransaction{ExternalAccountID: "ACT-1", ID: "TRN-1", Amount: "-12.50", Posted: 1755900000, Payee: "Blue Bottle", Description: "COFFEE SHOP", Raw: raw})
	ev, err := simplefin.Parse(payload, berlin)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalAccountID != "ACT-1" || ev.ExternalTransactionID != "TRN-1" || ev.Type != model.TransactionTypeExpense || ev.Amount != "12.5" || ev.Payee != "Blue Bottle" || ev.Description != "COFFEE SHOP" {
		t.Fatalf("ev = %+v", ev)
	}
	if got := ev.PostedAt.Format("2006-01-02 15:04:05"); got != "2025-08-23 00:00:00" { // 1755900000 = 2025-08-22T22:00:00Z
		t.Fatalf("posted = %s", got)
	}
}

func TestParse_IncomeAndPayeeFallback(t *testing.T) {
	payload := imports.EncodePullEvent(model.ExternalTransaction{ExternalAccountID: "ACT-1", ID: "TRN-2", Amount: "2500.00", Posted: 1755910000, Description: "PAYROLL",
		Raw: json.RawMessage(`{"id":"TRN-2","posted":1755910000,"amount":"2500.00","description":"PAYROLL","payee":""}`)})
	ev, err := simplefin.Parse(payload, time.UTC)
	if err != nil || ev.Type != model.TransactionTypeIncome || ev.Amount != "2500" || ev.Payee != "PAYROLL" {
		t.Fatalf("ev = %+v err %v", ev, err)
	}
}

func TestParse_Rejects(t *testing.T) {
	for name, raw := range map[string]string{
		"zero amount": `{"id":"T","posted":1755910000,"amount":"0"}`,
		"bad amount":  `{"id":"T","posted":1755910000,"amount":"12,50 EUR"}`,
		"no id":       `{"posted":1755910000,"amount":"1"}`,
		"no posted":   `{"id":"T","amount":"1"}`,
		"zero posted": `{"id":"T","posted":0,"amount":"1"}`,
		// the bridge client forwards rows it could not decode itself, so the
		// parser is what records them as failed
		"posted not a number": `{"id":"T","posted":"yesterday","amount":"1"}`,
		"blank id":            `{"id":"","posted":1755910000,"amount":"1"}`,
		"not json":            `nope`,
	} {
		payload := []byte(`{"externalAccountId":"ACT-1","transaction":` + func() string {
			if raw == "nope" {
				return `"nope"`
			}
			return raw
		}() + `}`)
		if _, err := simplefin.Parse(payload, time.UTC); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := simplefin.Parse([]byte(`{"transaction":{"id":"T","posted":1,"amount":"1"}}`), time.UTC); err == nil {
		t.Error("missing externalAccountId must fail")
	}
}
