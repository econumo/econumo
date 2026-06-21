package tag_test

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
	tagID1 = "aaaaaaaa-0000-0000-0000-000000000001"
	tagID2 = "aaaaaaaa-0000-0000-0000-000000000002"
	tagID3 = "aaaaaaaa-0000-0000-0000-000000000003"
)

func createReq(id, name string) map[string]any {
	return map[string]any{"id": id, "name": name}
}

// itemWrapper / itemsWrapper are the {item} / {items} data shapes.
type itemWrapper struct {
	Item tagItem `json:"item"`
}
type itemsWrapper struct {
	Items []tagItem `json:"items"`
}

func TestCreateTag_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#shopping"))
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
	if it.ID != tagID1 {
		t.Fatalf("item.id = %q, want %q", it.ID, tagID1)
	}
	if it.OwnerUserID != seedUserID {
		t.Fatalf("item.ownerUserId = %q, want %q", it.OwnerUserID, seedUserID)
	}
	if it.Name != "#shopping" {
		t.Fatalf("item.name = %q, want #shopping", it.Name)
	}
	if it.Position != 0 {
		t.Fatalf("first tag position = %d, want 0", it.Position)
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

	// The tag result must NOT carry a type or icon field (unlike category).
	itemObj := mustObject(t, probe["item"])
	if _, ok := itemObj["type"]; ok {
		t.Fatalf("tag item must not have a type field; body: %s", env.raw)
	}
	if _, ok := itemObj["icon"]; ok {
		t.Fatalf("tag item must not have an icon field; body: %s", env.raw)
	}
	// isArchived must serialize as a JSON number, not a bool.
	if string(itemObj["isArchived"]) != "0" {
		t.Fatalf("isArchived raw = %s, want numeric 0", itemObj["isArchived"])
	}
}

func mustObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode object: %v", err)
	}
	return m
}

func TestCreateTag_PositionIncrements(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#one"))
	_, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID2, "#two"))

	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.Position != 1 {
		t.Fatalf("second tag position = %d, want 1", res.Item.Position)
	}
}

func TestCreateTag_ShortName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "ab"))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	msgs, ok := env.errorsMap()["name"]
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected a name field error; body: %s", env.raw)
	}
	if msgs[0] != "Tag name must be 3-64 characters" {
		t.Fatalf("name error = %q, want exact 'Tag name must be 3-64 characters'", msgs[0])
	}
}

func TestCreateTag_DuplicateName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	if st, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#dup")); st != http.StatusOK {
		t.Fatalf("first create status = %d; body: %s", st, env.raw)
	}
	// A different id but the same name -> rejected by the uniqueness check.
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID2, "#dup"))
	if status != http.StatusBadRequest {
		t.Fatalf("duplicate-name create status = %d, want 400; body: %s", status, env.raw)
	}
}

func TestCreateTag_NoToken_401(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", "", createReq(tagID1, "#shopping"))
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
}

func TestCreateTag_DuplicateId_Idempotency_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	if st, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#first")); st != http.StatusOK {
		t.Fatalf("first create status = %d; body: %s", st, env.raw)
	}
	// Same request id again (different name) -> rejected by the operation-id guard.
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#other"))
	if status != http.StatusBadRequest {
		t.Fatalf("duplicate-id create status = %d, want 400; body: %s", status, env.raw)
	}
}

func TestUpdateTag_ChangesName(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#food"))

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/update-tag", token, map[string]any{
		"id": tagID1, "name": "#groceries",
	})
	if status != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.Name != "#groceries" {
		t.Fatalf("returned name = %q, want #groceries", res.Item.Name)
	}

	// Verify persistence via get-tag-list.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/tag/get-tag-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].Name != "#groceries" {
		t.Fatalf("persisted list = %+v, want one item named #groceries", list.Items)
	}
}

func TestUpdateTag_DuplicateName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedTag(t, tagID1, seedUserID, "#a", 0, false)
	h.seedTag(t, tagID2, seedUserID, "#b", 1, false)

	// Renaming #b to #a collides.
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/update-tag", token, map[string]any{
		"id": tagID2, "name": "#a",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (duplicate name); body: %s", status, env.raw)
	}
}

func TestUpdateTag_SameName_Allowed(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedTag(t, tagID1, seedUserID, "#keep", 0, false)

	// Updating a tag to its own current name must NOT trip the uniqueness check.
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/update-tag", token, map[string]any{
		"id": tagID1, "name": "#keep",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
}

func TestArchiveTag_ThenListShowsArchived(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#food"))

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/archive-tag", token, map[string]any{"id": tagID1})
	if status != http.StatusOK {
		t.Fatalf("archive status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.IsArchived != 1 {
		t.Fatalf("archived item.isArchived = %d, want 1", res.Item.IsArchived)
	}

	// get-tag-list returns archived tags too, with isArchived=1.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/tag/get-tag-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].IsArchived != 1 {
		t.Fatalf("list = %+v, want one item with isArchived=1", list.Items)
	}

	// Unarchive flips it back.
	_, unEnv := h.do(t, http.MethodPost, "/api/v1/tag/unarchive-tag", token, map[string]any{"id": tagID1})
	un := mustUnmarshal[itemWrapper](t, unEnv.Data)
	if un.Item.IsArchived != 0 {
		t.Fatalf("unarchived item.isArchived = %d, want 0", un.Item.IsArchived)
	}
}

func TestOrderTagList_Reorders(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#first"))  // pos 0
	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID2, "#second")) // pos 1
	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID3, "#third"))  // pos 2

	// Reverse: tagID3 -> 0, tagID1 -> 2.
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/order-tag-list", token, map[string]any{
		"changes": []map[string]any{
			{"id": tagID3, "position": 0},
			{"id": tagID1, "position": 2},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("order status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	if len(res.Items) != 3 {
		t.Fatalf("returned %d items, want 3", len(res.Items))
	}
	pos := map[string]int{}
	for _, it := range res.Items {
		pos[it.ID] = it.Position
	}
	if pos[tagID3] != 0 || pos[tagID2] != 1 || pos[tagID1] != 2 {
		t.Fatalf("positions = %v, want tagID3=0 tagID2=1 tagID1=2", pos)
	}
	if res.Items[0].ID != tagID3 || res.Items[2].ID != tagID1 {
		t.Fatalf("list order = [%s,...,%s], want [%s,...,%s]", res.Items[0].ID, res.Items[2].ID, tagID3, tagID1)
	}
}

func TestOrderTagList_Empty_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/order-tag-list", token, map[string]any{
		"changes": []map[string]any{},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (empty changes); body: %s", status, env.raw)
	}
}

func TestGetTagList_OrderedByPosition(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// Seed out-of-order positions directly.
	h.seedTag(t, tagID1, seedUserID, "#c", 2, false)
	h.seedTag(t, tagID2, seedUserID, "#a", 0, false)
	h.seedTag(t, tagID3, seedUserID, "#b", 1, true) // archived, still listed

	status, env := h.do(t, http.MethodGet, "/api/v1/tag/get-tag-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	if len(res.Items) != 3 {
		t.Fatalf("returned %d items, want 3", len(res.Items))
	}
	if res.Items[0].ID != tagID2 || res.Items[1].ID != tagID3 || res.Items[2].ID != tagID1 {
		t.Fatalf("order = [%s,%s,%s], want [%s,%s,%s] (by position)",
			res.Items[0].ID, res.Items[1].ID, res.Items[2].ID, tagID2, tagID3, tagID1)
	}
	if res.Items[1].IsArchived != 1 {
		t.Fatalf("archived tag isArchived = %d, want 1", res.Items[1].IsArchived)
	}
}

func TestDeleteTag_RemovesIt(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/tag/create-tag", token, createReq(tagID1, "#food"))

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/delete-tag", token, map[string]any{"id": tagID1})
	if status != http.StatusOK {
		t.Fatalf("delete status = %d, want 200; body: %s", status, env.raw)
	}
	// Result data is an empty object ({}).
	probe := mustObject(t, env.Data)
	if len(probe) != 0 {
		t.Fatalf("delete data = %s, want empty object", env.Data)
	}

	// get-tag-list now empty.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/tag/get-tag-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 0 {
		t.Fatalf("list after delete = %+v, want empty", list.Items)
	}
}

func TestUpdateTag_NotOwned_403(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// Tag owned by the OTHER user.
	h.seedTag(t, tagID1, otherUserID, "#theirs", 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/update-tag", token, map[string]any{
		"id": tagID1, "name": "#hijacked",
	})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
}

func TestDeleteTag_NotOwned_403(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedTag(t, tagID1, otherUserID, "#theirs", 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/delete-tag", token, map[string]any{"id": tagID1})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
}
