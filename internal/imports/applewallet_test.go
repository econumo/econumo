package imports

import (
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

var received = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

func TestNormalizeAmount(t *testing.T) {
	cases := []struct{ in, want string }{
		{"12.50", "12.50"}, {"$12.50", "12.50"}, {"12,50 €", "12.50"}, {"1.234,56", "1234.56"},
		{"1,234.56", "1234.56"}, {"-12.50", "12.50"}, {"12", "12"}, {"1,234", "1.234"}, {" 7 ", "7"}, {".5", "0.5"}, {"1.2.3.", "123"},
	}
	for _, c := range cases {
		got, err := normalizeAmount(c.in)
		if err != nil {
			t.Errorf("normalizeAmount(%q): %v", c.in, err)
			continue
		}
		if !vo.NewDecimal(got).Equals(vo.NewDecimal(c.want)) {
			t.Errorf("normalizeAmount(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "abc", "0", "0.00", "$"} {
		if _, err := normalizeAmount(bad); err == nil {
			t.Errorf("normalizeAmount(%q) must fail", bad)
		}
	}
}

func TestParseAppleWalletEvent_Full(t *testing.T) {
	body := []byte(`{"account":"  Apple   Card ","payee":" Blue Bottle ","amount":"$4.75","currency":"usd","occurredAt":"2026-08-15T10:42:03-07:00","type":"Expense","eventId":"evt-1"}`)
	ev, err := ParseAppleWalletEvent(body, received)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalAccountID != "Apple Card" || ev.Payee != "Blue Bottle" || ev.Currency != "USD" || ev.ExternalTransactionID != "evt-1" || ev.Type != model.TransactionTypeExpense || ev.Description != "" {
		t.Errorf("parsed = %+v", ev)
	}
	if !vo.NewDecimal(ev.Amount).Equals(vo.NewDecimal("4.75")) {
		t.Errorf("amount = %q", ev.Amount)
	}
	want := time.Date(2026, 8, 15, 10, 42, 3, 0, time.FixedZone("", -7*3600))
	if !ev.PostedAt.Equal(want) || ev.PostedAt.Format("15:04") != "10:42" {
		t.Errorf("postedAt = %v (offset must be preserved)", ev.PostedAt)
	}
}

func TestParseAppleWalletEvent_DefaultsAndSynthesizedID(t *testing.T) {
	body := []byte(`{"account":"Apple Card","payee":"Shop","amount":12.5,"currency":"EUR"}`)
	ev, err := ParseAppleWalletEvent(body, received)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != model.TransactionTypeExpense || !ev.PostedAt.Equal(received) {
		t.Errorf("defaults: %+v", ev)
	}
	if len(ev.ExternalTransactionID) != 64 {
		t.Errorf("synthesized id = %q, want sha256 hex", ev.ExternalTransactionID)
	}
	// the synthesized id is a function of the PARSED values, so formatting noise does not fork it
	again, _ := ParseAppleWalletEvent([]byte(`{"account":" Apple  Card","payee":" Shop ","amount":"12,50","currency":"eur"}`), received)
	if again.ExternalTransactionID != ev.ExternalTransactionID {
		t.Errorf("synthesized id must be stable across formatting: %q vs %q", again.ExternalTransactionID, ev.ExternalTransactionID)
	}
	// income flips the type; an unparsable occurredAt falls back to receivedAt
	inc, err := ParseAppleWalletEvent([]byte(`{"account":"Apple Card","amount":"1","currency":"USD","type":"income","occurredAt":"yesterday"}`), received)
	if err != nil || inc.Type != model.TransactionTypeIncome || !inc.PostedAt.Equal(received) {
		t.Errorf("income/fallback: %+v, %v", inc, err)
	}
}

func TestParseAppleWalletEvent_Errors(t *testing.T) {
	cases := map[string]string{
		`not json`:                        "invalid JSON",
		`{"amount":"1","currency":"USD"}`: "account is required",
		`{"account":"Apple Card","amount":"zero","currency":"USD"}`:         "amount must be a positive number",
		`{"account":"Apple Card","amount":"1","currency":"$"}`:              "currency must be a 3-letter code",
		`{"account":"Apple Card","amount":"1"}`:                             "currency must be a 3-letter code",
		`{"account":"Apple Card","amount":"1","currency":"USD","type":"x"}`: "type must be expense or income",
	}
	for body, want := range cases {
		_, err := ParseAppleWalletEvent([]byte(body), received)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", body, err, want)
		}
	}
}
