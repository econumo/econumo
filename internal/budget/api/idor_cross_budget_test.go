package api_test

// Cross-budget IDOR regression: a budget's folder/envelope child mutators
// role-check the budget named in the REQUEST, then reach the child by its own
// UUID. Without a membership guard, an attacker who owns any budget of their own
// (trivially passing canUpdate/canDelete on it) could rename or delete another
// budget's folders/envelopes just by knowing their ids (which get-budget exposes
// to every shared member, guests included). These tests drive the real router:
// the attacker passes THEIR budget id + the victim's child id and must be denied,
// and the victim's row must be untouched.

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	attackerBudgetID = "bbbb2222-0000-7000-8000-0000000000a1"
	victimFolderID   = "bf000000-0000-7000-8000-0000000000f1"
	victimEnvID      = "eeee2222-0000-7000-8000-0000000000e1"
	attackerAcctID   = "aaaa9999-0000-7000-8000-0000000000a1"
)

// seedAttackerBudget adds the second user and a budget they own (seeded
// directly, so it needs no accounts/categories), returning the attacker's token.
func seedAttackerBudget(t *testing.T, h *harness) string {
	t.Helper()
	other := h.seedSecondUser(t)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Budget(fixture.Budget{ID: attackerBudgetID, UserID: secondUserID, CurrencyID: usdID, Name: "Attacker"})
	return other
}

// seedVictimFolder creates a folder in the owner's budget via the real API.
func seedVictimFolder(t *testing.T, h *harness, tok string) {
	t.Helper()
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": victimFolderID, "name": "Victim Bills",
	}); st != http.StatusOK {
		t.Fatalf("create-folder precondition=%d body=%s", st, e.raw)
	}
}

// seedVictimEnvelope creates an envelope in the owner's budget via the real API.
func seedVictimEnvelope(t *testing.T, h *harness, tok string) {
	t.Helper()
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": victimEnvID, "name": "Victim Env", "icon": "wallet",
		"currencyId": usdID, "categories": []string{catID},
	}); st != http.StatusOK {
		t.Fatalf("create-envelope precondition=%d body=%s", st, e.raw)
	}
}

func TestUpdateFolder_ForeignBudget_DeniedAndUnchanged(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner)
	seedVictimFolder(t, h, owner)
	attacker := seedAttackerBudget(t, h)

	// Attacker references THEIR budget but the VICTIM's folder id.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-folder", attacker, map[string]any{
		"budgetId": attackerBudgetID, "id": victimFolderID, "name": "HACKED",
	})
	assertBudgetDenied(t, status, env)

	var name string
	h.db.QueryRow(`SELECT name FROM budgets_folders WHERE id = ?`, victimFolderID).Scan(&name)
	if name != "Victim Bills" {
		t.Fatalf("victim folder name=%q want %q (mutated across budgets)", name, "Victim Bills")
	}
}

func TestDeleteFolder_ForeignBudget_DeniedAndPreserved(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner)
	seedVictimFolder(t, h, owner)
	attacker := seedAttackerBudget(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/delete-folder", attacker, map[string]any{
		"budgetId": attackerBudgetID, "id": victimFolderID,
	})
	assertBudgetDenied(t, status, env)

	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_folders WHERE id = ?`, victimFolderID).Scan(&n)
	if n != 1 {
		t.Fatalf("victim folder rows=%d want 1 (deleted across budgets)", n)
	}
}

func TestUpdateEnvelope_ForeignBudget_DeniedAndUnchanged(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner)
	seedVictimEnvelope(t, h, owner)
	attacker := seedAttackerBudget(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", attacker, map[string]any{
		"budgetId": attackerBudgetID, "id": victimEnvID, "name": "HACKED", "icon": "skull",
		"currencyId": usdID, "isArchived": 1, "categories": []string{},
	})
	assertBudgetDenied(t, status, env)

	var name string
	var archived int
	h.db.QueryRow(`SELECT name, is_archived FROM budgets_envelopes WHERE id = ?`, victimEnvID).Scan(&name, &archived)
	if name != "Victim Env" || archived != 0 {
		t.Fatalf("victim envelope name=%q archived=%d want %q/0 (mutated across budgets)", name, archived, "Victim Env")
	}
}

func TestDeleteEnvelope_ForeignBudget_DeniedAndPreserved(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner)
	seedVictimEnvelope(t, h, owner)
	attacker := seedAttackerBudget(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/delete-envelope", attacker, map[string]any{
		"budgetId": attackerBudgetID, "id": victimEnvID,
	})
	assertBudgetDenied(t, status, env)

	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes WHERE id = ?`, victimEnvID).Scan(&n)
	if n != 1 {
		t.Fatalf("victim envelope rows=%d want 1 (deleted across budgets)", n)
	}
}

// A client-supplied folder id that already exists (here, in the victim's budget)
// must not be reusable on create — the upsert would otherwise overwrite the
// existing row rather than create a new one.
func TestCreateFolder_ReusedForeignId_DeniedAndUnchanged(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner)
	seedVictimFolder(t, h, owner)
	attacker := seedAttackerBudget(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", attacker, map[string]any{
		"budgetId": attackerBudgetID, "id": victimFolderID, "name": "Stolen",
	})
	assertBudgetDenied(t, status, env)

	var name, budgetID string
	h.db.QueryRow(`SELECT name, budget_id FROM budgets_folders WHERE id = ?`, victimFolderID).Scan(&name, &budgetID)
	if name != "Victim Bills" || budgetID != budgetID1 {
		t.Fatalf("victim folder name=%q budget=%q want %q/%q (overwritten via create)", name, budgetID, "Victim Bills", budgetID1)
	}
}

func TestCreateEnvelope_ReusedForeignId_DeniedAndUnchanged(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner)
	seedVictimEnvelope(t, h, owner)
	attacker := seedAttackerBudget(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", attacker, map[string]any{
		"budgetId": attackerBudgetID, "id": victimEnvID, "name": "Stolen", "icon": "skull",
		"currencyId": usdID, "categories": []string{},
	})
	assertBudgetDenied(t, status, env)

	var name string
	h.db.QueryRow(`SELECT name FROM budgets_envelopes WHERE id = ?`, victimEnvID).Scan(&name)
	if name != "Victim Env" {
		t.Fatalf("victim envelope name=%q want %q (overwritten via create)", name, "Victim Env")
	}
}

// A budget may only be shared with a connected user; granting to a stranger
// (existing but not connected) is denied.
func TestBudgetGrantAccess_UnconnectedUser_Denied(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	const strangerID = "44444444-4444-4444-4444-444444444444"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: strangerID, Name: "Stranger", Avatar: "https://avatar.test/s", Salt: seedSalt})

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": strangerID, "role": "user",
	})
	assertBudgetDenied(t, status, env)
}

// include/exclude-account must require write access to the BUDGET, not just
// ownership of the account being toggled.
func TestExcludeAccount_ForeignBudget_Denied(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t)
	seedBudget(t, h, owner) // owner's budgetID1
	attacker := h.seedSecondUser(t)
	// The attacker owns an account of their own but has no access to budgetID1.
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Account(fixture.Account{ID: attackerAcctID, UserID: secondUserID, CurrencyID: usdID, Name: "Mine"})

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", attacker, map[string]any{
		"id": budgetID1, "accountId": attackerAcctID,
	})
	assertBudgetDenied(t, status, env)

	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id = ?`, budgetID1).Scan(&n)
	if n != 0 {
		t.Fatalf("excluded-account rows=%d want 0 (wrote to a foreign budget)", n)
	}
}
