package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// usdSeedID is the baseline migration's global USD row, seeded on every fresh
// database (see harness.resetCurrencies).
const usdSeedID = "dffc2a06-6f29-4704-8575-31709adee926"

type currencyItemWrapper struct {
	Item currencyListItem `json:"item"`
}
type currencyItemsWrapper struct {
	Items []currencyItem `json:"items"`
}
type currencyListItem struct {
	currencyItem
	Scope      string `json:"scope"`
	IsArchived int    `json:"isArchived"`
	IsHidden   int    `json:"isHidden"`
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

func TestCreateCurrency_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/create-currency", token, map[string]any{
		"id": "aaaaaaaa-0000-0000-0000-000000000001", "code": "PTS", "name": "Points", "symbol": "pts",
		"fractionDigits": 0, "rate": "100",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[currencyItemWrapper](t, env.Data)
	if res.Item.Code != "PTS" || res.Item.Name != "Points" || res.Item.Symbol != "pts" || res.Item.FractionDigits != 0 {
		t.Fatalf("item = %+v, want PTS/Points/pts/0", res.Item)
	}
	if res.Item.Scope != "own" || res.Item.IsArchived != 0 || res.Item.IsHidden != 0 {
		t.Fatalf("item scope/flags = %+v, want own/0/0", res.Item)
	}
	if res.Item.ID == "" {
		t.Fatalf("item id must be set; body: %s", env.raw)
	}
}

func TestCreateCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/create-currency", "", map[string]any{
		"id": "aaaaaaaa-0000-0000-0000-000000000002", "code": "PTS", "name": "Points",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}

func TestCreateCurrency_DuplicateCode_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	body := map[string]any{"id": "aaaaaaaa-0000-0000-0000-000000000003", "code": "PTS", "name": "Points"}
	if status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/create-currency", token, body); status != http.StatusOK {
		t.Fatalf("first create status = %d, want 200; body: %s", status, env.raw)
	}

	dup := map[string]any{"id": "aaaaaaaa-0000-0000-0000-000000000004", "code": "PTS", "name": "Points again"}
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/create-currency", token, dup)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
	errs := env.errorsMap()
	if len(errs["code"]) == 0 || errs["code"][0] != "Currency already exists" {
		t.Fatalf("errors[code] = %v, want ['Currency already exists']; body: %s", errs["code"], env.raw)
	}
}

func TestUpdateCurrency_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000001", seedUserID, "PTS", "pts", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/update-currency", token, map[string]any{
		"id": "bbbbbbbb-0000-0000-0000-000000000001", "name": "Kid points", "symbol": "kp", "fractionDigits": 3,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[currencyItemWrapper](t, env.Data)
	if res.Item.Name != "Kid points" || res.Item.Symbol != "kp" || res.Item.FractionDigits != 3 {
		t.Fatalf("item = %+v, want Kid points/kp/3", res.Item)
	}
}

func TestUpdateCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000002", seedUserID, "PTS", "pts", 2)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/update-currency", "", map[string]any{
		"id": "bbbbbbbb-0000-0000-0000-000000000002", "name": "X", "symbol": "x", "fractionDigits": 2,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}

func TestUpdateCurrency_Foreign_403(t *testing.T) {
	h := newHarness(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000003", otherUserID, "PTS", "pts", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/update-currency", seedUserID, map[string]any{
		"id": "bbbbbbbb-0000-0000-0000-000000000003", "name": "Hijack", "symbol": "x", "fractionDigits": 2,
	})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
}

func TestArchiveUnarchiveCurrency_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000004", seedUserID, "PTS", "pts", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/archive-currency", token, map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000004"})
	if status != http.StatusOK {
		t.Fatalf("archive status = %d, want 200; body: %s", status, env.raw)
	}

	listStatus, listEnv := h.do(t, http.MethodGet, "/api/v1/currency/get-currency-list", token)
	if listStatus != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listStatus, listEnv.raw)
	}
	list := mustUnmarshal[currencyItemsWrapperFull](t, listEnv.Data)
	found := false
	for _, it := range list.Items {
		if it.ID == "bbbbbbbb-0000-0000-0000-000000000004" {
			found = true
			if it.IsArchived != 1 {
				t.Fatalf("isArchived = %d, want 1 after archive; body: %s", it.IsArchived, listEnv.raw)
			}
		}
	}
	if !found {
		t.Fatalf("archived currency missing from list; body: %s", listEnv.raw)
	}

	status, env = h.doJSON(t, http.MethodPost, "/api/v1/currency/unarchive-currency", token, map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000004"})
	if status != http.StatusOK {
		t.Fatalf("unarchive status = %d, want 200; body: %s", status, env.raw)
	}
}

func TestArchiveCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000005", seedUserID, "PTS", "pts", 2)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/archive-currency", "", map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000005"})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}

type currencyItemsWrapperFull struct {
	Items []currencyListItem `json:"items"`
}

func TestDeleteCurrency_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000006", seedUserID, "PTS", "pts", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/delete-currency", token, map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000006"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
}

func TestDeleteCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000007", seedUserID, "PTS", "pts", 2)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/delete-currency", "", map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000007"})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}

func TestDeleteCurrency_InUse_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000008", seedUserID, "PTS", "pts", 2)
	h.f.Account(fixture.Account{UserID: seedUserID, CurrencyID: "bbbbbbbb-0000-0000-0000-000000000008"})

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/delete-currency", token, map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000008"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if env.Message != "Currency is in use and cannot be deleted" {
		t.Fatalf("message = %q, want 'Currency is in use and cannot be deleted'; body: %s", env.Message, env.raw)
	}
}

func TestSetCurrencyRate_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-0000-000000000009", seedUserID, "PTS", "pts", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/set-currency-rate", token, map[string]any{
		"currencyId": "bbbbbbbb-0000-0000-0000-000000000009", "rate": "120.5", "date": "2026-01-15",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
}

func TestSetCurrencyRate_NoToken_401(t *testing.T) {
	h := newHarness(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-000-00000000000a", seedUserID, "PTS", "pts", 2)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/set-currency-rate", "", map[string]any{
		"currencyId": "bbbbbbbb-0000-0000-000-00000000000a", "rate": "1",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}

func TestSetCurrencyRate_Global_403(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/set-currency-rate", token, map[string]any{
		"currencyId": usdSeedID, "rate": "2",
	})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
}

func TestHideShowCurrency_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.resetCurrencies(t)
	h.seedCurrency(t, eurID, "EUR", "€", 2)
	// The stub profile currency defaults to EUR, so hide a currency the caller
	// isn't using: RUB.
	h.seedCurrency(t, rubID, "RUB", "₽", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/hide-currency", token, map[string]any{"id": rubID})
	if status != http.StatusOK {
		t.Fatalf("hide status = %d, want 200; body: %s", status, env.raw)
	}

	status, env = h.doJSON(t, http.MethodPost, "/api/v1/currency/show-currency", token, map[string]any{"id": rubID})
	if status != http.StatusOK {
		t.Fatalf("show status = %d, want 200; body: %s", status, env.raw)
	}
}

func TestHideCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/hide-currency", "", map[string]any{"id": usdSeedID})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}

func TestHideCurrency_Custom_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedCustomCurrency(t, "bbbbbbbb-0000-0000-000-00000000000b", seedUserID, "PTS", "pts", 2)

	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/hide-currency", token, map[string]any{"id": "bbbbbbbb-0000-0000-000-00000000000b"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if env.Message != "This currency cannot be hidden" {
		t.Fatalf("message = %q, want 'This currency cannot be hidden'; body: %s", env.Message, env.raw)
	}
}

func TestShowCurrency_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, env := h.doJSON(t, http.MethodPost, "/api/v1/currency/show-currency", "", map[string]any{"id": usdSeedID})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
}
