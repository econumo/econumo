package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	sideIncomeCatID  = "cccc4444-0000-7000-8000-000000000001"
	sideExpenseCatID = "cccc4444-0000-7000-8000-000000000002"
	sideFolderA      = "bfff4444-0000-7000-8000-000000000001"
	sideFolderB      = "bfff4444-0000-7000-8000-000000000002"
)

func storedFolderSide(t *testing.T, h *harness, folderID string) string {
	t.Helper()
	var side string
	if err := h.db.QueryRow(`SELECT side FROM budgets_folders WHERE id = ?`, folderID).Scan(&side); err != nil {
		t.Fatalf("read folder side: %v", err)
	}
	return side
}

func createFolder(t *testing.T, h *harness, tok, budgetID, id string, side any) (int, envelope) {
	t.Helper()
	body := map[string]any{"budgetId": budgetID, "id": id, "name": "Folder " + id[len(id)-2:]}
	if side != nil {
		body["side"] = side
	}
	return h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, body)
}

func TestCreateFolder_Side(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Folder Side Budget"))

	cases := []struct {
		id   string
		side any
		want string
	}{
		{"bfff4444-0000-7000-8000-000000000011", nil, "expense"},
		{"bfff4444-0000-7000-8000-000000000012", "", "expense"},
		{"bfff4444-0000-7000-8000-000000000013", "expense", "expense"},
		{"bfff4444-0000-7000-8000-000000000014", "income", "income"},
	}
	for _, c := range cases {
		st, env := createFolder(t, h, tok, budgetID1, c.id, c.side)
		if st != http.StatusOK {
			t.Fatalf("create-folder side=%v = %d; body=%s", c.side, st, env.raw)
		}
		res := mustUnmarshal[model.CreateBudgetFolderResult](t, env.Data)
		if res.Item.Side != c.want {
			t.Errorf("create-folder side=%v: response side %q, want %q", c.side, res.Item.Side, c.want)
		}
		if got := storedFolderSide(t, h, c.id); got != c.want {
			t.Errorf("create-folder side=%v: stored side %q, want %q", c.side, got, c.want)
		}
	}

	const badID = "bfff4444-0000-7000-8000-000000000015"
	st, env := createFolder(t, h, tok, budgetID1, badID, "savings")
	if st != http.StatusBadRequest {
		t.Fatalf("create-folder side=savings = %d, want 400; body=%s", st, env.raw)
	}
	if msgs := env.errorsMap()["side"]; len(msgs) == 0 || msgs[0] != "The value you selected is not a valid choice." {
		t.Errorf("want the invalid-choice error on side; body=%s", env.raw)
	}
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_folders WHERE id = ?`, badID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rejected folder must not be written: n=%d err=%v", n, err)
	}
}

func TestGetBudget_EmptyFolderVisibilityFollowsStoredSide(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Folder Side Budget"))
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": budgetID1, "id": sideFolderA, "name": "Earnings", "side": "income"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": budgetID1, "id": sideFolderB, "name": "Bills"})

	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	res := mustUnmarshal[model.GetBudgetResult](t, env.Data)
	if len(res.Item.Structure.Folders) != 1 || res.Item.Structure.Folders[0].Id != sideFolderB {
		t.Fatalf("get-budget folders = %+v, want only the empty expense folder", res.Item.Structure.Folders)
	}
	if res.Item.Structure.Folders[0].Side != "expense" {
		t.Errorf("expense folder side on the wire = %q", res.Item.Structure.Folders[0].Side)
	}
}

func TestMoveElement_EmptyFolderAdoptsSide(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.f.Category(fixture.Category{ID: sideIncomeCatID, UserID: seedUserID, Name: "Wages Side", Type: 1, Icon: "payments"})
	h.f.Category(fixture.Category{ID: sideExpenseCatID, UserID: seedUserID, Name: "Rent Side", Type: 0, Icon: "home"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Folder Side Budget"))
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": budgetID1, "id": sideFolderA, "name": "Starts Expense"})

	// The first income element into an empty expense folder is accepted and
	// turns the folder into an income folder.
	h.mustDo(t, http.MethodPost, "/api/v1/budget/move-element", tok,
		map[string]any{"budgetId": budgetID1, "id": sideIncomeCatID, "folderId": sideFolderA, "afterId": nil})
	if got := storedFolderSide(t, h, sideFolderA); got != "income" {
		t.Fatalf("folder side after the first income member = %q, want income", got)
	}

	// Now non-empty and income: an expense element is refused.
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok,
		map[string]any{"budgetId": budgetID1, "id": sideExpenseCatID, "folderId": sideFolderA, "afterId": nil})
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "A folder cannot contain both income and expenses") {
		t.Fatalf("expense into non-empty income folder: st=%d body=%s", st, env.raw)
	}

	// Emptied, it keeps its side: still hidden from get-budget, listed as
	// income by get-budget-plan.
	h.mustDo(t, http.MethodPost, "/api/v1/budget/move-element", tok,
		map[string]any{"budgetId": budgetID1, "id": sideIncomeCatID, "folderId": nil, "afterId": nil})
	if got := storedFolderSide(t, h, sideFolderA); got != "income" {
		t.Fatalf("emptied folder side = %q, want income", got)
	}
	env = h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if strings.Contains(string(env.Data), sideFolderA) {
		t.Fatalf("an empty income folder must not render in get-budget; body=%s", env.Data)
	}
	st, env = getPlan(t, h, tok, "id="+budgetID1+"&months=1")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	plan := planItem(t, env)
	if len(plan.Structure.Folders) != 1 || plan.Structure.Folders[0].Side != "income" {
		t.Fatalf("plan folders = %+v, want the one income folder", plan.Structure.Folders)
	}

	// An empty folder accepts the other side again, and adopts it.
	h.mustDo(t, http.MethodPost, "/api/v1/budget/move-element", tok,
		map[string]any{"budgetId": budgetID1, "id": sideExpenseCatID, "folderId": sideFolderA, "afterId": nil})
	if got := storedFolderSide(t, h, sideFolderA); got != "expense" {
		t.Fatalf("folder side after an expense member = %q, want expense", got)
	}
}

func TestCreateEnvelope_FolderSideAndOwnership(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Folder Side Budget"))
	const otherBudgetID = "bbbb4444-0000-7000-8000-000000000002"
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(otherBudgetID, "Other Budget"))
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": budgetID1, "id": sideFolderA, "name": "Was Income", "side": "income"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": otherBudgetID, "id": sideFolderB, "name": "Foreign"})

	// Another budget's folder is refused, and nothing is written.
	const foreignEnvID = "beee4444-0000-7000-8000-000000000001"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": foreignEnvID, "name": "Foreign Env", "icon": "i",
		"currencyId": usdID, "folderId": sideFolderB, "categories": []string{},
	})
	if st != http.StatusForbidden {
		t.Fatalf("create-envelope into another budget's folder = %d, want 403; body=%s", st, env.raw)
	}
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes WHERE id = ?`, foreignEnvID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused envelope must not be written: n=%d err=%v", n, err)
	}

	// An expense envelope into an empty income folder: accepted, the folder
	// adopts the expense side.
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": "beee4444-0000-7000-8000-000000000002", "name": "Home Env", "icon": "i",
		"currencyId": usdID, "folderId": sideFolderA, "categories": []string{},
	})
	if got := storedFolderSide(t, h, sideFolderA); got != "expense" {
		t.Fatalf("folder side after an expense envelope = %q, want expense", got)
	}
}

func TestCloneBudget_CopiesFolderSide(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": cloneSrcID, "name": "Src", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": cloneSrcID, "id": sideFolderA, "name": "Earnings", "side": "income"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": cloneSrcID, "id": sideFolderB, "name": "Bills"})

	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Copy", "withLimits": false})

	rows, err := h.db.Query(`SELECT name, side FROM budgets_folders WHERE budget_id = ? ORDER BY name`, cloneDstID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var name, side string
		if err := rows.Scan(&name, &side); err != nil {
			t.Fatal(err)
		}
		got[name] = side
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["Earnings"] != "income" || got["Bills"] != "expense" {
		t.Fatalf("cloned folder sides = %v, want Earnings=income Bills=expense", got)
	}
}
