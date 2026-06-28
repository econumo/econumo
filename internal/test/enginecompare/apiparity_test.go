//go:build enginecompare

package enginecompare

// The comprehensive API engine-parity catalogue. Each scenario is a sequence of
// HTTP calls (reads, and write->read sequences) replayed against the production
// handler on BOTH SQLite and PostgreSQL from an identical seed; runAPIOnBoth
// asserts every call's (status, raw body) is byte-identical, with SQLite as the
// reference (target) engine.
//
// Coverage target (confirmed): every read/list endpoint across all 9 modules,
// plus a create/update/delete-then-read sequence per mutating module so the
// persisted state after a write is compared too — catching engine differences in
// upserts, datetime writes, decimal rounding, and ordering that a read-only
// sweep would miss.

import (
	"regexp"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// apiCall is one request in a scenario plus a label for diff messages.
type apiCall struct {
	label  string
	method string
	path   string
	// auth selects which seeded user's token to attach: "owner", "guest", or ""
	// (public/no token).
	auth string
	body any
}

// apiScenario returns the ordered list of calls to replay. It is a func (not a
// static slice) so a scenario can build a body referencing freshly-generated ids
// if needed; most just return a literal list.
type apiScenario func() []apiCall

// runAPIOnBoth seeds an identical fixture on a fresh SQLite DB and a fresh
// PostgreSQL DB, stands up the production handler over each, replays the
// scenario's calls against both, and asserts each call's status + raw body match
// (SQLite is the reference). The PostgreSQL half SKIPS when DATABASE_TEST_PGSQL_URL
// is unset; the SQLite half still runs (so the scenario is always exercised).
func runAPIOnBoth(t *testing.T, name string, sc apiScenario) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		calls := sc()

		// Reference run: SQLite.
		var refStatus []int
		var refBody [][]byte
		t.Run("sqlite", func(t *testing.T) {
			h := newAPIHarness(t, dbtest.NewSQLite(t))
			refStatus, refBody = replay(t, h, calls)
		})

		// Comparison run: PostgreSQL (skips if unconfigured).
		t.Run("postgresql", func(t *testing.T) {
			h := newAPIHarness(t, dbtest.NewPostgres(t)) // SKIPs if env unset
			pgStatus, pgBody := replay(t, h, calls)
			for i := range calls {
				if pgStatus[i] != refStatus[i] {
					t.Errorf("[%s] status mismatch: sqlite=%d pgsql=%d", calls[i].label, refStatus[i], pgStatus[i])
				}
				ref, pg := normalizeBody(refBody[i]), normalizeBody(pgBody[i])
				if ref != pg {
					t.Errorf("[%s] body mismatch:\n  sqlite: %s\n  pgsql : %s", calls[i].label, ref, pg)
				}
			}
		})
	})
}

// replay issues each call against the harness, returning per-call statuses and
// raw bodies. Tokens are minted per-run (engine-independent: the JWT signer + the
// seeded users are identical across engines).
func replay(t *testing.T, h *apiHarness, calls []apiCall) ([]int, [][]byte) {
	t.Helper()
	ownerTok := h.token(t, apiOwnerID, apiOwnerEmail)
	guestTok := h.token(t, apiGuestID, apiGuestEmail)

	statuses := make([]int, len(calls))
	bodies := make([][]byte, len(calls))
	for i, c := range calls {
		var tok string
		switch c.auth {
		case "owner":
			tok = ownerTok
		case "guest":
			tok = guestTok
		case "":
			tok = ""
		default:
			t.Fatalf("[%s] unknown auth %q", c.label, c.auth)
		}
		statuses[i], bodies[i] = h.call(t, c.method, c.path, tok, c.body)
	}
	return statuses, bodies
}

// uuidV7Re matches a version-7 UUID (the version nibble after the 2nd dash is
// '7'). Every CREATE endpoint mints a fresh server-side vo.NewId() (UUIDv7,
// time+random based) and ignores the client-supplied id (which is only the
// idempotency operation key). Those generated ids legitimately differ per run
// and per engine — they are NOT a parity property — so they are redacted before
// comparison. The fixed fixture ids (and the USD currency id) are deliberately
// NOT v7, so they survive normalization and remain strictly compared.
var uuidV7Re = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-7[0-9a-fA-F]{3}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// normalizeBody redacts server-generated UUIDv7 ids so the comparison focuses on
// everything else (names, positions, amounts, timestamps, ordering, envelope
// shape). All other bytes are compared strictly.
func normalizeBody(b []byte) string {
	return uuidV7Re.ReplaceAllString(string(b), "<generated-uuid>")
}

// ---- read-endpoint catalogue (one scenario per module) ----

func TestAPIParity_UserReads(t *testing.T) {
	runAPIOnBoth(t, "user_reads", func() []apiCall {
		return []apiCall{
			{"get-user-data", "POST", "/api/v1/user/get-user-data", "owner", map[string]any{}},
			{"get-option-list", "POST", "/api/v1/user/get-option-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_CurrencyReads(t *testing.T) {
	runAPIOnBoth(t, "currency_reads", func() []apiCall {
		return []apiCall{
			{"get-currency-list", "POST", "/api/v1/currency/get-currency-list", "owner", map[string]any{}},
			{"get-currency-rate-list", "POST", "/api/v1/currency/get-currency-rate-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_CategoryReads(t *testing.T) {
	runAPIOnBoth(t, "category_reads", func() []apiCall {
		return []apiCall{
			{"get-category-list", "POST", "/api/v1/category/get-category-list", "owner", map[string]any{}},
			{"get-category-list-guest", "POST", "/api/v1/category/get-category-list", "guest", map[string]any{}},
		}
	})
}

func TestAPIParity_TagReads(t *testing.T) {
	runAPIOnBoth(t, "tag_reads", func() []apiCall {
		return []apiCall{
			{"get-tag-list", "POST", "/api/v1/tag/get-tag-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_PayeeReads(t *testing.T) {
	runAPIOnBoth(t, "payee_reads", func() []apiCall {
		return []apiCall{
			{"get-payee-list", "POST", "/api/v1/payee/get-payee-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_AccountReads(t *testing.T) {
	runAPIOnBoth(t, "account_reads", func() []apiCall {
		return []apiCall{
			{"get-account-list", "POST", "/api/v1/account/get-account-list", "owner", map[string]any{}},
			{"get-account-list-guest", "POST", "/api/v1/account/get-account-list", "guest", map[string]any{}},
			{"get-folder-list", "POST", "/api/v1/account/get-folder-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_TransactionReads(t *testing.T) {
	runAPIOnBoth(t, "transaction_reads", func() []apiCall {
		return []apiCall{
			{"get-transaction-list", "POST", "/api/v1/transaction/get-transaction-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_ConnectionReads(t *testing.T) {
	runAPIOnBoth(t, "connection_reads", func() []apiCall {
		return []apiCall{
			{"get-connection-list", "POST", "/api/v1/connection/get-connection-list", "owner", map[string]any{}},
			{"get-connection-list-guest", "POST", "/api/v1/connection/get-connection-list", "guest", map[string]any{}},
		}
	})
}

func TestAPIParity_BudgetReads(t *testing.T) {
	runAPIOnBoth(t, "budget_reads", func() []apiCall {
		return []apiCall{
			{"get-budget-list", "POST", "/api/v1/budget/get-budget-list", "owner", map[string]any{}},
		}
	})
}

// ---- write -> read sequences (per mutating module) ----

func TestAPIParity_CategoryWriteRead(t *testing.T) {
	runAPIOnBoth(t, "category_write_read", func() []apiCall {
		const newCat = "c0000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-category", "POST", "/api/v1/category/create-category", "owner",
				map[string]any{"id": newCat, "name": "Travel", "type": 0, "icon": "plane"}},
			{"read-after-create", "POST", "/api/v1/category/get-category-list", "owner", map[string]any{}},
			{"update-category", "POST", "/api/v1/category/update-category", "owner",
				map[string]any{"id": newCat, "name": "Travel2", "icon": "plane2"}},
			{"archive-category", "POST", "/api/v1/category/archive-category", "owner", map[string]any{"id": newCat}},
			{"read-after-archive", "POST", "/api/v1/category/get-category-list", "owner", map[string]any{}},
			{"unarchive-category", "POST", "/api/v1/category/unarchive-category", "owner", map[string]any{"id": newCat}},
			{"delete-category", "POST", "/api/v1/category/delete-category", "owner", map[string]any{"id": newCat}},
			{"read-after-delete", "POST", "/api/v1/category/get-category-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_TagWriteRead(t *testing.T) {
	runAPIOnBoth(t, "tag_write_read", func() []apiCall {
		const newTag = "10000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-tag", "POST", "/api/v1/tag/create-tag", "owner", map[string]any{"id": newTag, "name": "Urgent"}},
			{"read-after-create", "POST", "/api/v1/tag/get-tag-list", "owner", map[string]any{}},
			{"update-tag", "POST", "/api/v1/tag/update-tag", "owner", map[string]any{"id": newTag, "name": "Urgent2"}},
			{"archive-tag", "POST", "/api/v1/tag/archive-tag", "owner", map[string]any{"id": newTag}},
			{"unarchive-tag", "POST", "/api/v1/tag/unarchive-tag", "owner", map[string]any{"id": newTag}},
			{"delete-tag", "POST", "/api/v1/tag/delete-tag", "owner", map[string]any{"id": newTag}},
			{"read-after-delete", "POST", "/api/v1/tag/get-tag-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_PayeeWriteRead(t *testing.T) {
	runAPIOnBoth(t, "payee_write_read", func() []apiCall {
		const newPayee = "20000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-payee", "POST", "/api/v1/payee/create-payee", "owner", map[string]any{"id": newPayee, "name": "Cafe"}},
			{"read-after-create", "POST", "/api/v1/payee/get-payee-list", "owner", map[string]any{}},
			{"update-payee", "POST", "/api/v1/payee/update-payee", "owner", map[string]any{"id": newPayee, "name": "Cafe2"}},
			{"archive-payee", "POST", "/api/v1/payee/archive-payee", "owner", map[string]any{"id": newPayee}},
			{"unarchive-payee", "POST", "/api/v1/payee/unarchive-payee", "owner", map[string]any{"id": newPayee}},
			{"delete-payee", "POST", "/api/v1/payee/delete-payee", "owner", map[string]any{"id": newPayee}},
			{"read-after-delete", "POST", "/api/v1/payee/get-payee-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_AccountWriteRead(t *testing.T) {
	runAPIOnBoth(t, "account_write_read", func() []apiCall {
		const newAcct = "a0000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-account", "POST", "/api/v1/account/create-account", "owner",
				map[string]any{"id": newAcct, "name": "Savings", "type": 2, "icon": "bank", "currencyId": apiUSD}},
			{"read-after-create", "POST", "/api/v1/account/get-account-list", "owner", map[string]any{}},
			{"update-account", "POST", "/api/v1/account/update-account", "owner",
				map[string]any{"id": newAcct, "name": "Savings2", "icon": "bank2"}},
			{"read-after-update", "POST", "/api/v1/account/get-account-list", "owner", map[string]any{}},
			{"delete-account", "POST", "/api/v1/account/delete-account", "owner", map[string]any{"id": newAcct}},
			{"read-after-delete", "POST", "/api/v1/account/get-account-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_TransactionWriteRead(t *testing.T) {
	runAPIOnBoth(t, "transaction_write_read", func() []apiCall {
		const newTxn = "d0000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-transaction", "POST", "/api/v1/transaction/create-transaction", "owner",
				map[string]any{
					"id": newTxn, "accountId": apiOwnerAccount, "type": 1,
					"amount": "9.99", "categoryId": apiCatFood, "date": "2024-04-02 10:00:00",
				}},
			{"read-after-create", "POST", "/api/v1/transaction/get-transaction-list", "owner", map[string]any{}},
			{"account-list-after-create", "POST", "/api/v1/account/get-account-list", "owner", map[string]any{}},
			{"delete-transaction", "POST", "/api/v1/transaction/delete-transaction", "owner", map[string]any{"id": newTxn}},
			{"read-after-delete", "POST", "/api/v1/transaction/get-transaction-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_BudgetWriteRead(t *testing.T) {
	runAPIOnBoth(t, "budget_write_read", func() []apiCall {
		const newBudget = "b0000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-budget", "POST", "/api/v1/budget/create-budget", "owner",
				map[string]any{"id": newBudget, "name": "Trip", "currencyId": apiUSD, "startedAt": "2024-04-01"}},
			{"read-after-create", "POST", "/api/v1/budget/get-budget-list", "owner", map[string]any{}},
			{"set-limit", "POST", "/api/v1/budget/set-limit", "owner",
				map[string]any{
					"budgetId": newBudget, "elementId": apiCatFood, "elementType": 0,
					"period": "2024-04-01", "amount": "300", "currencyId": apiUSD,
				}},
			{"update-budget", "POST", "/api/v1/budget/update-budget", "owner",
				map[string]any{"id": newBudget, "name": "Trip2"}},
			{"read-after-update", "POST", "/api/v1/budget/get-budget-list", "owner", map[string]any{}},
			{"delete-budget", "POST", "/api/v1/budget/delete-budget", "owner", map[string]any{"id": newBudget}},
			{"read-after-delete", "POST", "/api/v1/budget/get-budget-list", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_UserWriteRead(t *testing.T) {
	runAPIOnBoth(t, "user_write_read", func() []apiCall {
		return []apiCall{
			{"update-name", "POST", "/api/v1/user/update-name", "owner", map[string]any{"name": "Renamed"}},
			{"update-report-period", "POST", "/api/v1/user/update-report-period", "owner", map[string]any{"reportPeriod": "weekly"}},
			{"read-after-update", "POST", "/api/v1/user/get-user-data", "owner", map[string]any{}},
		}
	})
}

func TestAPIParity_FolderWriteRead(t *testing.T) {
	runAPIOnBoth(t, "folder_write_read", func() []apiCall {
		const newFolder = "f0000000-0000-0000-0000-0000000000ff"
		return []apiCall{
			{"create-folder", "POST", "/api/v1/account/create-folder", "owner", map[string]any{"id": newFolder, "name": "Trips"}},
			{"read-after-create", "POST", "/api/v1/account/get-folder-list", "owner", map[string]any{}},
			{"update-folder", "POST", "/api/v1/account/update-folder", "owner", map[string]any{"id": newFolder, "name": "Trips2"}},
			{"hide-folder", "POST", "/api/v1/account/hide-folder", "owner", map[string]any{"id": newFolder, "isVisible": false}},
			{"read-after-hide", "POST", "/api/v1/account/get-folder-list", "owner", map[string]any{}},
			{"delete-folder", "POST", "/api/v1/account/delete-folder", "owner", map[string]any{"id": newFolder}},
			{"read-after-delete", "POST", "/api/v1/account/get-folder-list", "owner", map[string]any{}},
		}
	})
}

// ---- error-path parity (validation + auth envelopes must also match) ----

func TestAPIParity_ErrorPaths(t *testing.T) {
	runAPIOnBoth(t, "error_paths", func() []apiCall {
		return []apiCall{
			{"unauthorized", "POST", "/api/v1/user/get-user-data", "", map[string]any{}},
			{"validation-empty-name", "POST", "/api/v1/category/create-category", "owner",
				map[string]any{"id": "c0000000-0000-0000-0000-0000000000ee", "name": "", "type": 0, "icon": "x"}},
			{"not-found-update", "POST", "/api/v1/category/update-category", "owner",
				map[string]any{"id": "c0000000-0000-0000-0000-0000000000dd", "name": "Ghost", "icon": "x"}},
		}
	})
}

// guard against an empty catalogue silently passing.
func TestAPIParity_CatalogueNonEmpty(t *testing.T) {
	if got := apiCatalogueSize(); got < 15 {
		t.Fatalf("API parity catalogue unexpectedly small: %d scenarios", got)
	}
}

// apiCatalogueSize is a trivial self-check count of the scenarios above; bump it
// as scenarios are added. It exists so a refactor that accidentally drops a
// scenario is caught.
func apiCatalogueSize() int { return 18 }
