package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	cloneSrcID      = "bbbb3333-0000-7000-8000-000000000001"
	cloneDstID      = "bbbb3333-0000-7000-8000-000000000002"
	cloneDstID2     = "bbbb3333-0000-7000-8000-000000000003"
	cloneFolderID   = "bfff3333-0000-7000-8000-000000000001"
	cloneEnvelopeID = "beee3333-0000-7000-8000-000000000001"
)

// metaOut reads {item: BudgetResult}, whose meta nests one level deeper than
// the {item: MetaResult} responses metaItemOut covers.
type metaOut struct {
	Item struct {
		Meta struct {
			Id         string `json:"id"`
			Name       string `json:"name"`
			StartedAt  string `json:"startedAt"`
			EndedAt    string `json:"endedAt"`
			IsArchived int    `json:"isArchived"`
		} `json:"meta"`
	} `json:"item"`
}

func TestCloneBudget_FullCopyEquivalence(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": cloneSrcID, "name": "Src", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": cloneSrcID, "id": cloneFolderID, "name": "Folder"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		map[string]any{"budgetId": cloneSrcID, "id": cloneEnvelopeID, "name": "Envelope", "icon": "i", "currencyId": usdID, "categories": []string{catID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": cloneSrcID, "elementId": tagID, "period": "2026-03-01", "amount": "100"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": cloneSrcID, "elementId": tagID, "period": "2026-07-01", "amount": "50"})

	// share it with an accepted participant so access copying is observable
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok,
		map[string]any{"budgetId": cloneSrcID, "userId": otherUserID, "role": "user"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": cloneSrcID})

	env := h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Copy", "withLimits": true})
	meta := mustUnmarshal[metaOut](t, env.Data)
	if meta.Item.Meta.Id != cloneDstID || meta.Item.Meta.Name != "Copy" {
		t.Fatalf("copy meta=%+v", meta.Item.Meta)
	}
	// the copy starts open-ended, unarchived, at the source's start month
	if meta.Item.Meta.EndedAt != "" || meta.Item.Meta.IsArchived != 0 {
		t.Fatalf("copy carries lifecycle state: %+v", meta.Item.Meta)
	}
	if meta.Item.Meta.StartedAt != "2026-01-01 00:00:00" {
		t.Fatalf("copy startedAt=%q want the source start", meta.Item.Meta.StartedAt)
	}

	var n int
	q := func(sql string, args ...any) int {
		t.Helper()
		if err := h.db.QueryRow(sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := q(`SELECT COUNT(*) FROM budgets_access WHERE budget_id = ?`, cloneDstID); got != 1 {
		t.Fatalf("access rows=%d want 1", got)
	}
	var accepted bool
	if err := h.db.QueryRow(`SELECT is_accepted FROM budgets_access WHERE budget_id = ? AND user_id = ?`, cloneDstID, otherUserID).Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("accepted state not copied")
	}
	if got := q(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, cloneDstID, accountID); got != 1 {
		t.Fatal("membership not copied")
	}
	if got := q(`SELECT COUNT(*) FROM budgets_folders WHERE budget_id = ?`, cloneDstID); got != 1 {
		t.Fatalf("folders=%d want 1", got)
	}

	// the envelope is re-created under a FRESH id, and the envelope element's
	// external_id points at that new id (not the source's).
	var newEnvID string
	if err := h.db.QueryRow(`SELECT id FROM budgets_envelopes WHERE budget_id = ?`, cloneDstID).Scan(&newEnvID); err != nil {
		t.Fatal(err)
	}
	if newEnvID == "" || newEnvID == cloneEnvelopeID {
		t.Fatalf("envelope id not fresh: %q", newEnvID)
	}
	if got := q(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND type = 0 AND external_id = ?`, cloneDstID, newEnvID); got != 1 {
		t.Fatal("envelope element external_id not remapped to the new envelope id")
	}
	// the envelope's category links come along
	if got := q(`SELECT COUNT(*) FROM budgets_envelopes_categories WHERE budget_envelope_id = ?`, newEnvID); got != 1 {
		t.Fatalf("envelope category links=%d want 1", got)
	}
	// full backup copies every limit
	if got := q(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ?`, cloneDstID); got != 2 {
		t.Fatalf("limits=%d want 2 (full backup copies all)", got)
	}
	// element ids are all fresh
	if got := q(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND id IN (SELECT id FROM budgets_elements WHERE budget_id = ?)`, cloneDstID, cloneSrcID); got != 0 {
		t.Fatalf("%d element ids shared with the source", got)
	}
}

func TestCloneBudget_StartDateAndLimitsFilters(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": cloneSrcID, "name": "Src", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": cloneSrcID, "elementId": catID, "period": "2026-03-01", "amount": "100"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": cloneSrcID, "elementId": catID, "period": "2026-08-01", "amount": "50"})

	countLimits := func(budgetID string) int {
		t.Helper()
		var n int
		if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ?`, budgetID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// continuation: startDate = this month → only the August limit copies
	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Cont", "startDate": "2026-08-01", "withLimits": true})
	if got := countLimits(cloneDstID); got != 1 {
		t.Fatalf("limits=%d want 1 (period >= startDate only)", got)
	}

	// withLimits=false → none
	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID2, "name": "Bare", "withLimits": false})
	if got := countLimits(cloneDstID2); got != 0 {
		t.Fatalf("limits=%d want 0", got)
	}

	// startDate before the source start → 400; after this month → 400
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": vo.NewId().String(), "name": "Early", "startDate": "2025-12-01", "withLimits": false})
	if st != http.StatusBadRequest {
		t.Fatalf("pre-start startDate accepted: status=%d body=%s", st, env.raw)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": vo.NewId().String(), "name": "Late", "startDate": "2026-09-01", "withLimits": false})
	if st != http.StatusBadRequest {
		t.Fatalf("future startDate accepted: status=%d body=%s", st, env.raw)
	}
}

func TestCloneBudget_AdminBecomesOwnerOfCopy(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(cloneSrcID, "Src"))
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok,
		map[string]any{"budgetId": cloneSrcID, "userId": otherUserID, "role": "admin"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": cloneSrcID})

	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", otherUserID,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Copy", "withLimits": false})

	// the cloner owns the copy
	var copyOwner string
	if err := h.db.QueryRow(`SELECT user_id FROM budgets WHERE id = ?`, cloneDstID).Scan(&copyOwner); err != nil {
		t.Fatal(err)
	}
	if copyOwner != otherUserID {
		t.Fatalf("copy owner=%q want the cloning admin %q", copyOwner, otherUserID)
	}

	// sharing set: the cloner's own grant is dropped (ownership replaces it) and
	// the former owner joins as an accepted admin, so every participant — and
	// with them every member account — stays on the copy.
	var accessRows int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_access WHERE budget_id = ?`, cloneDstID).Scan(&accessRows); err != nil {
		t.Fatal(err)
	}
	if accessRows != 1 {
		t.Fatalf("access rows=%d want 1 (former owner only)", accessRows)
	}
	var role int
	var accepted bool
	if err := h.db.QueryRow(`SELECT role, is_accepted FROM budgets_access WHERE budget_id = ? AND user_id = ?`, cloneDstID, seedUserID).Scan(&role, &accepted); err != nil {
		t.Fatalf("former owner has no access row on the copy: %v", err)
	}
	if role != 0 || !accepted {
		t.Fatalf("former owner role=%d accepted=%v want accepted admin (0)", role, accepted)
	}

	// membership: the former owner's account is still a member of the copy
	var memberRows int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, cloneDstID, accountID).Scan(&memberRows); err != nil {
		t.Fatal(err)
	}
	if memberRows != 1 {
		t.Fatal("former owner's account membership not copied")
	}
}

func TestCloneBudget_EditorDenied_ArchivedSourceAllowed(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(cloneSrcID, "Src"))
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok,
		map[string]any{"budgetId": cloneSrcID, "userId": otherUserID, "role": "user"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": cloneSrcID})

	// only owner|admin may clone — an editor ("user" role) is refused
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", otherUserID,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Nope", "withLimits": false})
	if st != http.StatusForbidden {
		t.Fatalf("editor clone: status=%d body=%s want 403", st, env.raw)
	}

	// an archived source is still cloneable by its owner
	h.mustDo(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": cloneSrcID})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Copy", "withLimits": false})
}
