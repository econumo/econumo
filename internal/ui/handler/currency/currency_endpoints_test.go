package currency_test

import (
	"net/http"
	"testing"
)

type currencyItemsWrapper struct {
	Items []currencyItem `json:"items"`
}
type rateItemsWrapper struct {
	Items []rateItem `json:"items"`
}

func TestGetCurrencyList_OrderedByCode_NameFromIntl(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.resetCurrencies(t)

	// Seed out of code order; the endpoint must return them code-ASC. All names
	// are NULL in the DB, so the wire name comes from the Intl table by code.
	h.seedCurrency(t, rubID, "RUB", "₽", 2)
	h.seedCurrency(t, usdID, "USD", "$", 2)
	h.seedCurrency(t, eurID, "EUR", "€", 2)

	status, env := h.do(t, http.MethodGet, "/api/v1/currency/get-currency-list", token)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[currencyItemsWrapper](t, env.Data)
	if len(res.Items) != 3 {
		t.Fatalf("got %d currencies, want 3; body: %s", len(res.Items), env.raw)
	}

	// Ordered by code ASC: EUR, RUB, USD.
	if res.Items[0].Code != "EUR" || res.Items[1].Code != "RUB" || res.Items[2].Code != "USD" {
		t.Fatalf("order = [%s,%s,%s], want [EUR,RUB,USD]", res.Items[0].Code, res.Items[1].Code, res.Items[2].Code)
	}

	// Names resolved from the Intl table (byte-exact display names).
	byCode := map[string]currencyItem{}
	for _, it := range res.Items {
		byCode[it.Code] = it
	}
	if byCode["USD"].Name != "US Dollar" {
		t.Fatalf("USD name = %q, want 'US Dollar'", byCode["USD"].Name)
	}
	if byCode["EUR"].Name != "Euro" {
		t.Fatalf("EUR name = %q, want 'Euro'", byCode["EUR"].Name)
	}
	if byCode["RUB"].Name != "Russian Ruble" {
		t.Fatalf("RUB name = %q, want 'Russian Ruble'", byCode["RUB"].Name)
	}

	// Other fields pass through.
	if byCode["USD"].ID != usdID || byCode["USD"].Symbol != "$" || byCode["USD"].FractionDigits != 2 {
		t.Fatalf("USD item = %+v, want id/symbol/fractionDigits from seed", byCode["USD"])
	}
}

func TestGetCurrencyList_UnknownCode_FallsBackToCode(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.resetCurrencies(t)

	// A code with no Intl entry must fall back to the code itself. "ZZZ" is not a
	// real ISO code.
	h.seedCurrency(t, usdID, "ZZZ", "?", 0)

	_, env := h.do(t, http.MethodGet, "/api/v1/currency/get-currency-list", token)
	res := mustUnmarshal[currencyItemsWrapper](t, env.Data)
	if len(res.Items) != 1 || res.Items[0].Name != "ZZZ" {
		t.Fatalf("items = %+v, want one item with name 'ZZZ' (code fallback)", res.Items)
	}
}

func TestGetCurrencyList_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/currency/get-currency-list", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
}

func TestGetCurrencyRateList_LatestDateOnly(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.resetCurrencies(t)

	h.seedCurrency(t, usdID, "USD", "$", 2)
	h.seedCurrency(t, eurID, "EUR", "€", 2)
	h.seedCurrency(t, rubID, "RUB", "₽", 2)

	// Two publish dates. Only the latest (2025-12-14) must be returned.
	h.seedRate(t, "r1", eurID, usdID, "2025-12-01", "0.90000000")
	h.seedRate(t, "r2", rubID, usdID, "2025-12-01", "90.00000000")
	h.seedRate(t, "r3", eurID, usdID, "2025-12-14", "0.92000000")
	h.seedRate(t, "r4", rubID, usdID, "2025-12-14", "95.00000000")

	status, env := h.do(t, http.MethodGet, "/api/v1/currency/get-currency-rate-list", token)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[rateItemsWrapper](t, env.Data)
	if len(res.Items) != 2 {
		t.Fatalf("got %d rates, want 2 (latest date only); body: %s", len(res.Items), env.raw)
	}
	for _, it := range res.Items {
		if it.UpdatedAt != "2025-12-14 00:00:00" {
			t.Fatalf("rate updatedAt = %q, want '2025-12-14 00:00:00'", it.UpdatedAt)
		}
		if it.BaseCurrencyID != usdID {
			t.Fatalf("rate baseCurrencyId = %q, want %q", it.BaseCurrencyID, usdID)
		}
	}
	// Rates are normalized to the decimal wire form (trailing zeros trimmed):
	// stored "0.92000000" -> "0.92", "95.00000000" -> "95".
	byCur := map[string]rateItem{}
	for _, it := range res.Items {
		byCur[it.CurrencyID] = it
	}
	if byCur[eurID].Rate != "0.92" {
		t.Fatalf("EUR rate = %q, want '0.92' (normalized)", byCur[eurID].Rate)
	}
	if byCur[rubID].Rate != "95" {
		t.Fatalf("RUB rate = %q, want '95' (normalized)", byCur[rubID].Rate)
	}
}

func TestGetCurrencyRateList_Empty(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// No rates at all -> empty items list (not null), 200.
	status, env := h.do(t, http.MethodGet, "/api/v1/currency/get-currency-rate-list", token)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[rateItemsWrapper](t, env.Data)
	if res.Items == nil {
		t.Fatalf("items must be an empty array, not null; body: %s", env.raw)
	}
	if len(res.Items) != 0 {
		t.Fatalf("got %d rates, want 0; body: %s", len(res.Items), env.raw)
	}
}
