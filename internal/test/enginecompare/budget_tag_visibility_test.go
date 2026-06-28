//go:build enginecompare

package enginecompare

// A tag given a budget limit but with NO transactions must stay visible in
// get-budget on BOTH engines. This is the engine-parity guard for the
// budget-tag-visibility fix (builder_structure_build.go): the structure builder
// used to gate tags on having a transaction-derived spending entry, dropping a
// budgeted-but-unspent tag. The handler-level regression runs on SQLite; this
// pins the same behavior on the PostgreSQL adapter production uses.
//
// Synthetic data only: dbtest provisions a throwaway, randomly-named database.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestBudgetTagWithLimitNoTransactions_PerEngine(t *testing.T) {
	run := func(t *testing.T, db *dbtest.DB) {
		h := newAPIHarness(t, db)
		tok := h.token(t, apiOwnerID, apiOwnerEmail)
		const budgetID = "b0000000-0000-0000-0000-0000000000aa"

		// Fresh budget with a known start so the period math is deterministic.
		if st, body := h.call(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
			map[string]any{"id": budgetID, "name": "Tag Vis", "currencyId": apiUSD, "startDate": "2024-04-01"}); st != http.StatusOK {
			t.Fatalf("create-budget = %d; body: %s", st, body)
		}

		// Limit on the seeded tag (apiTagWork has NO transactions in the fixture).
		if st, body := h.call(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
			map[string]any{"budgetId": budgetID, "elementId": apiTagWork, "period": "2024-04-01", "amount": "300"}); st != http.StatusOK {
			t.Fatalf("set-limit = %d; body: %s", st, body)
		}

		// get-budget for that month must include the tag with its limit.
		st, body := h.call(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID+"&date=2024-04-15", tok, nil)
		if st != http.StatusOK {
			t.Fatalf("get-budget = %d; body: %s", st, body)
		}
		var env struct {
			Data struct {
				Item struct {
					Structure struct {
						Elements []struct {
							Id       string `json:"id"`
							Budgeted string `json:"budgeted"`
						} `json:"elements"`
					} `json:"structure"`
				} `json:"item"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode get-budget (%s): %v\nbody: %s", db.Engine, err, body)
		}
		var found bool
		var budgeted string
		for _, e := range env.Data.Item.Structure.Elements {
			if e.Id == apiTagWork {
				found, budgeted = true, e.Budgeted
			}
		}
		if !found {
			t.Fatalf("[%s] tag with a limit but no transactions is missing from get-budget; body: %s", db.Engine, body)
		}
		if budgeted != "300" {
			t.Errorf("[%s] tag budgeted = %q, want 300", db.Engine, budgeted)
		}
	}

	t.Run("sqlite", func(t *testing.T) { run(t, dbtest.NewSQLite(t)) })
	t.Run("postgresql", func(t *testing.T) { run(t, dbtest.NewPostgres(t)) }) // SKIPs if env unset
}
