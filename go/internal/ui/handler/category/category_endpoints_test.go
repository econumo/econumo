package category_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
)

// apiDatetime matches the API datetime format "2006-01-02 15:04:05" (space
// separator, no timezone).
var apiDatetime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

const (
	catID1 = "aaaaaaaa-0000-0000-0000-000000000001"
	catID2 = "aaaaaaaa-0000-0000-0000-000000000002"
	catID3 = "aaaaaaaa-0000-0000-0000-000000000003"
)

func createReq(id, name, typ string) map[string]any {
	return map[string]any{"id": id, "name": name, "type": typ}
}

// itemWrapper / itemsWrapper are the {item} / {items} data shapes.
type itemWrapper struct {
	Item categoryItem `json:"item"`
}
type itemsWrapper struct {
	Items []categoryItem `json:"items"`
}

func TestCreateCategory_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Food", "expense"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	if !env.Success {
		t.Fatalf("success=false; body: %s", env.raw)
	}

	// Exact data keys: must be {item: {...}}.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &probe); err != nil {
		t.Fatalf("decode data: %v; body: %s", err, env.raw)
	}
	if _, ok := probe["item"]; !ok {
		t.Fatalf("data must have an item key; body: %s", env.raw)
	}

	res := mustUnmarshal[itemWrapper](t, env.Data)
	it := res.Item
	if it.ID != catID1 {
		t.Fatalf("item.id = %q, want %q", it.ID, catID1)
	}
	if it.OwnerUserID != seedUserID {
		t.Fatalf("item.ownerUserId = %q, want %q", it.OwnerUserID, seedUserID)
	}
	if it.Name != "Food" {
		t.Fatalf("item.name = %q, want Food", it.Name)
	}
	if it.Type != "expense" {
		t.Fatalf("item.type = %q, want expense", it.Type)
	}
	if it.Position != 0 {
		t.Fatalf("first category position = %d, want 0", it.Position)
	}
	if it.Icon != "local_offer" {
		t.Fatalf("item.icon = %q, want local_offer (default)", it.Icon)
	}
	if it.IsArchived != 0 {
		t.Fatalf("item.isArchived = %d, want 0", it.IsArchived)
	}
	if !apiDatetime.MatchString(it.CreatedAt) {
		t.Fatalf("item.createdAt = %q, want 2006-01-02 15:04:05", it.CreatedAt)
	}
	if !apiDatetime.MatchString(it.UpdatedAt) {
		t.Fatalf("item.updatedAt = %q, want 2006-01-02 15:04:05", it.UpdatedAt)
	}

	// isArchived must serialize as a JSON number, not a bool.
	if rawIsArchived := extractField(t, probe["item"], "isArchived"); rawIsArchived != "0" {
		t.Fatalf("isArchived raw = %s, want numeric 0", rawIsArchived)
	}
}

// extractField returns the raw JSON of a named field for type assertions.
func extractField(t *testing.T, obj json.RawMessage, key string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(obj, &m); err != nil {
		t.Fatalf("decode object: %v", err)
	}
	return string(m[key])
}

func TestCreateCategory_PositionIncrements(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Food", "expense"))
	_, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID2, "Salary", "income"))

	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.Position != 1 {
		t.Fatalf("second category position = %d, want 1", res.Item.Position)
	}
	if res.Item.Type != "income" {
		t.Fatalf("type = %q, want income", res.Item.Type)
	}
}

func TestCreateCategory_ShortName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "ab", "expense"))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	msgs, ok := env.Errors["name"]
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected a name field error; body: %s", env.raw)
	}
	if msgs[0] != "Category name must be 3-64 characters" {
		t.Fatalf("name error = %q, want exact 'Category name must be 3-64 characters'", msgs[0])
	}
}

func TestCreateCategory_NoToken_401(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", "", createReq(catID1, "Food", "expense"))
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
}

func TestCreateCategory_DuplicateId_Idempotency_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	if st, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Food", "expense")); st != http.StatusOK {
		t.Fatalf("first create status = %d; body: %s", st, env.raw)
	}
	// Same request id again -> rejected by the operation-id guard.
	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Other", "expense"))
	if status != http.StatusBadRequest {
		t.Fatalf("duplicate create status = %d, want 400; body: %s", status, env.raw)
	}
}

func TestUpdateCategory_ChangesName(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Food", "expense"))

	status, env := h.do(t, http.MethodPost, "/api/v1/category/update-category", token, map[string]any{
		"id": catID1, "name": "Groceries", "icon": "shopping_cart",
	})
	if status != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.Name != "Groceries" {
		t.Fatalf("returned name = %q, want Groceries", res.Item.Name)
	}
	if res.Item.Icon != "shopping_cart" {
		t.Fatalf("returned icon = %q, want shopping_cart", res.Item.Icon)
	}

	// Verify persistence via get-category-list.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].Name != "Groceries" {
		t.Fatalf("persisted list = %+v, want one item named Groceries", list.Items)
	}
}

func TestArchiveCategory_ThenListShowsArchived(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Food", "expense"))

	status, env := h.do(t, http.MethodPost, "/api/v1/category/archive-category", token, map[string]any{"id": catID1})
	if status != http.StatusOK {
		t.Fatalf("archive status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.IsArchived != 1 {
		t.Fatalf("archived item.isArchived = %d, want 1", res.Item.IsArchived)
	}

	// get-category-list returns archived categories too, with isArchived=1.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].IsArchived != 1 {
		t.Fatalf("list = %+v, want one item with isArchived=1", list.Items)
	}

	// Unarchive flips it back.
	_, unEnv := h.do(t, http.MethodPost, "/api/v1/category/unarchive-category", token, map[string]any{"id": catID1})
	un := mustUnmarshal[itemWrapper](t, unEnv.Data)
	if un.Item.IsArchived != 0 {
		t.Fatalf("unarchived item.isArchived = %d, want 0", un.Item.IsArchived)
	}
}

func TestOrderCategoryList_Reorders(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "First", "expense"))  // pos 0
	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID2, "Second", "expense")) // pos 1
	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID3, "Third", "expense"))  // pos 2

	// Reverse the order: catID3 -> 0, catID1 -> 2.
	status, env := h.do(t, http.MethodPost, "/api/v1/category/order-category-list", token, map[string]any{
		"changes": []map[string]any{
			{"id": catID3, "position": 0},
			{"id": catID1, "position": 2},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("order status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	if len(res.Items) != 3 {
		t.Fatalf("returned %d items, want 3", len(res.Items))
	}
	// Items must come back ordered by position: catID3(0), catID2(1), catID1(2).
	pos := map[string]int{}
	for _, it := range res.Items {
		pos[it.ID] = it.Position
	}
	if pos[catID3] != 0 || pos[catID2] != 1 || pos[catID1] != 2 {
		t.Fatalf("positions = %v, want catID3=0 catID2=1 catID1=2", pos)
	}
	// And the returned slice itself must be position-ordered.
	if res.Items[0].ID != catID3 || res.Items[2].ID != catID1 {
		t.Fatalf("list order = [%s,...,%s], want [%s,...,%s]", res.Items[0].ID, res.Items[2].ID, catID3, catID1)
	}
}

func TestGetCategoryList_OrderedByPosition(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// Seed out-of-order positions directly.
	h.seedCategory(t, catID1, seedUserID, "C", 2, 0, false)
	h.seedCategory(t, catID2, seedUserID, "A", 0, 0, false)
	h.seedCategory(t, catID3, seedUserID, "B", 1, 0, true) // archived, still listed

	status, env := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	if len(res.Items) != 3 {
		t.Fatalf("returned %d items, want 3", len(res.Items))
	}
	if res.Items[0].ID != catID2 || res.Items[1].ID != catID3 || res.Items[2].ID != catID1 {
		t.Fatalf("order = [%s,%s,%s], want [%s,%s,%s] (by position)",
			res.Items[0].ID, res.Items[1].ID, res.Items[2].ID, catID2, catID3, catID1)
	}
	// The archived one (catID3) still appears with isArchived=1.
	if res.Items[1].IsArchived != 1 {
		t.Fatalf("archived category isArchived = %d, want 1", res.Items[1].IsArchived)
	}
}

func TestDeleteCategory_Delete_RemovesIt(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/category/create-category", token, createReq(catID1, "Food", "expense"))

	status, env := h.do(t, http.MethodPost, "/api/v1/category/delete-category", token, map[string]any{
		"id": catID1, "mode": "delete",
	})
	if status != http.StatusOK {
		t.Fatalf("delete status = %d, want 200; body: %s", status, env.raw)
	}
	// Result data is an empty object ({}).
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &probe); err != nil {
		t.Fatalf("decode data: %v; body: %s", err, env.raw)
	}
	if len(probe) != 0 {
		t.Fatalf("delete data = %s, want empty object", env.Data)
	}

	// get-category-list now empty.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 0 {
		t.Fatalf("list after delete = %+v, want empty", list.Items)
	}
}

func TestDeleteCategory_Replace_ReassignsAndRemoves(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// Two expense categories owned by the seeded user.
	h.seedCategory(t, catID1, seedUserID, "Old", 0, 0, false)
	h.seedCategory(t, catID2, seedUserID, "New", 1, 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/delete-category", token, map[string]any{
		"id": catID1, "mode": "replace", "replaceId": catID2,
	})
	if status != http.StatusOK {
		t.Fatalf("replace-delete status = %d, want 200; body: %s", status, env.raw)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].ID != catID2 {
		t.Fatalf("list after replace = %+v, want only catID2", list.Items)
	}
}

func TestUpdateCategory_NotOwned_403(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// Category owned by the OTHER user.
	h.seedCategory(t, catID1, otherUserID, "Theirs", 0, 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/update-category", token, map[string]any{
		"id": catID1, "name": "Hijacked", "icon": "local_offer",
	})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
}
