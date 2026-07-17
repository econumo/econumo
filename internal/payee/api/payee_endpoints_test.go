package api_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// apiDatetime matches the API datetime format "2006-01-02 15:04:05" (space
// separator, no timezone).
var apiDatetime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

const (
	payeeID1 = "aaaaaaaa-0000-0000-0000-000000000001"
	payeeID2 = "aaaaaaaa-0000-0000-0000-000000000002"
	payeeID3 = "aaaaaaaa-0000-0000-0000-000000000003"
)

func createReq(id, name string) map[string]any {
	return map[string]any{"id": id, "name": name}
}

// createPayee calls the create-payee endpoint and returns the minted entity id
// from the response, which is NOT equal to the operation/idempotency opID.
func createPayee(t *testing.T, h *harness, token, opID, name string) string {
	t.Helper()
	_, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(opID, name))
	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.ID == "" {
		t.Fatalf("create returned no id; body: %s", env.raw)
	}
	return res.Item.ID
}

type itemWrapper struct {
	Item payeeItem `json:"item"`
}
type itemsWrapper struct {
	Items []payeeItem `json:"items"`
}

func TestCreatePayee_Success(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID1, "Amazon"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	if !env.Success {
		t.Fatalf("success=false; body: %s", env.raw)
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &probe); err != nil {
		t.Fatalf("decode data: %v; body: %s", err, env.raw)
	}
	if _, ok := probe["item"]; !ok {
		t.Fatalf("data must have an item key; body: %s", env.raw)
	}

	res := mustUnmarshal[itemWrapper](t, env.Data)
	it := res.Item
	if it.ID == "" {
		t.Fatalf("item.id is empty; body: %s", env.raw)
	}
	if it.ID == payeeID1 {
		t.Fatalf("item.id = %q, must be a freshly minted id (not the request id %q)", it.ID, payeeID1)
	}
	if it.OwnerUserID != seedUserID {
		t.Fatalf("item.ownerUserId = %q, want %q", it.OwnerUserID, seedUserID)
	}
	if it.Name != "Amazon" {
		t.Fatalf("item.name = %q, want Amazon", it.Name)
	}
	if it.Position != 0 {
		t.Fatalf("first payee position = %d, want 0", it.Position)
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

	// The payee result must NOT carry a type or icon field.
	itemObj := mustObject(t, probe["item"])
	if _, ok := itemObj["type"]; ok {
		t.Fatalf("payee item must not have a type field; body: %s", env.raw)
	}
	if _, ok := itemObj["icon"]; ok {
		t.Fatalf("payee item must not have an icon field; body: %s", env.raw)
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

func TestCreatePayee_PositionIncrements(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID1, "One"))
	_, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID2, "Two"))

	res := mustUnmarshal[itemWrapper](t, env.Data)
	if res.Item.Position != 1 {
		t.Fatalf("second payee position = %d, want 1", res.Item.Position)
	}
}

func TestCreatePayee_ShortName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID1, "ab"))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	msgs, ok := env.errorsMap()["name"]
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected a name field error; body: %s", env.raw)
	}
	if msgs[0] != "Payee name must be 3-64 characters" {
		t.Fatalf("name error = %q, want exact 'Payee name must be 3-64 characters'", msgs[0])
	}
}

func TestCreatePayee_DuplicateName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	if st, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID1, "Dup")); st != http.StatusOK {
		t.Fatalf("first create status = %d; body: %s", st, env.raw)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID2, "Dup"))
	if status != http.StatusBadRequest {
		t.Fatalf("duplicate-name create status = %d, want 400; body: %s", status, env.raw)
	}
}

func TestCreatePayee_NoToken_401(t *testing.T) {
	h := newHarness(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", "", createReq(payeeID1, "Amazon"))
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("expected success=false; body: %s", env.raw)
	}
}

func TestCreatePayee_DuplicateId_Idempotency_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	if st, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID1, "First")); st != http.StatusOK {
		t.Fatalf("first create status = %d; body: %s", st, env.raw)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/payee/create-payee", token, createReq(payeeID1, "Other"))
	if status != http.StatusBadRequest {
		t.Fatalf("duplicate-id create status = %d, want 400; body: %s", status, env.raw)
	}
}

func TestUpdatePayee_ChangesName(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	id := createPayee(t, h, token, payeeID1, "Old")

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/update-payee", token, map[string]any{
		"id": id, "name": "New",
	})
	if status != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("update data = %s, want empty object {}", env.Data)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	var found *payeeItem
	for i := range list.Items {
		if list.Items[i].ID == id {
			found = &list.Items[i]
		}
	}
	if found == nil || found.Name != "New" {
		t.Fatalf("persisted list = %+v, want item %s named New", list.Items, id)
	}
}

func TestUpdatePayee_DuplicateName_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, seedUserID, "A", 0, false)
	h.seedPayee(t, payeeID2, seedUserID, "B", 1, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/update-payee", token, map[string]any{
		"id": payeeID2, "name": "A",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (duplicate name); body: %s", status, env.raw)
	}
}

func TestUpdatePayee_SameName_Allowed(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, seedUserID, "Keep", 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/update-payee", token, map[string]any{
		"id": payeeID1, "name": "Keep",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
}

func TestArchivePayee_ThenListShowsArchived(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	id := createPayee(t, h, token, payeeID1, "Shop")

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/archive-payee", token, map[string]any{"id": id})
	if status != http.StatusOK {
		t.Fatalf("archive status = %d, want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("archive data = %s, want empty object {}", env.Data)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].ID != id || list.Items[0].IsArchived != 1 {
		t.Fatalf("list = %+v, want one item %s with isArchived=1", list.Items, id)
	}

	_, unEnv := h.do(t, http.MethodPost, "/api/v1/payee/unarchive-payee", token, map[string]any{"id": id})
	if string(unEnv.Data) != "{}" {
		t.Fatalf("unarchive data = %s, want empty object {}", unEnv.Data)
	}

	_, listEnv2 := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	list2 := mustUnmarshal[itemsWrapper](t, listEnv2.Data)
	if len(list2.Items) != 1 || list2.Items[0].ID != id || list2.Items[0].IsArchived != 0 {
		t.Fatalf("list after unarchive = %+v, want one item %s with isArchived=0", list2.Items, id)
	}
}

func TestOrderPayeeList_Reorders(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	id1 := createPayee(t, h, token, payeeID1, "First")  // pos 0
	id2 := createPayee(t, h, token, payeeID2, "Second") // pos 1
	id3 := createPayee(t, h, token, payeeID3, "Third")  // pos 2

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/order-payee-list", token, map[string]any{
		"changes": []map[string]any{
			{"id": id3, "position": 0},
			{"id": id1, "position": 2},
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
	if pos[id3] != 0 || pos[id2] != 1 || pos[id1] != 2 {
		t.Fatalf("positions = %v, want id3=0 id2=1 id1=2", pos)
	}
	if res.Items[0].ID != id3 || res.Items[2].ID != id1 {
		t.Fatalf("list order = [%s,...,%s], want [%s,...,%s]", res.Items[0].ID, res.Items[2].ID, id3, id1)
	}
}

func TestOrderPayeeList_Empty_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/order-payee-list", token, map[string]any{
		"changes": []map[string]any{},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (empty changes); body: %s", status, env.raw)
	}
}

func TestGetPayeeList_OrderedByPosition(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, seedUserID, "C", 2, false)
	h.seedPayee(t, payeeID2, seedUserID, "A", 0, false)
	h.seedPayee(t, payeeID3, seedUserID, "B", 1, true) // archived, still listed

	status, env := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	if len(res.Items) != 3 {
		t.Fatalf("returned %d items, want 3", len(res.Items))
	}
	if res.Items[0].ID != payeeID2 || res.Items[1].ID != payeeID3 || res.Items[2].ID != payeeID1 {
		t.Fatalf("order = [%s,%s,%s], want [%s,%s,%s] (by position)",
			res.Items[0].ID, res.Items[1].ID, res.Items[2].ID, payeeID2, payeeID3, payeeID1)
	}
	if res.Items[1].IsArchived != 1 {
		t.Fatalf("archived payee isArchived = %d, want 1", res.Items[1].IsArchived)
	}
}

func TestDeletePayee_RemovesIt(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	id := createPayee(t, h, token, payeeID1, "Shop")

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/delete-payee", token, map[string]any{"id": id})
	if status != http.StatusOK {
		t.Fatalf("delete status = %d, want 200; body: %s", status, env.raw)
	}
	probe := mustObject(t, env.Data)
	if len(probe) != 0 {
		t.Fatalf("delete data = %s, want empty object", env.Data)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	list := mustUnmarshal[itemsWrapper](t, listEnv.Data)
	if len(list.Items) != 0 {
		t.Fatalf("list after delete = %+v, want empty", list.Items)
	}
}

func TestUpdatePayee_NotOwned_403(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, otherUserID, "Theirs", 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/update-payee", token, map[string]any{
		"id": payeeID1, "name": "Hijacked",
	})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
}

func TestDeletePayee_NotOwned_403(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, otherUserID, "Theirs", 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/delete-payee", token, map[string]any{"id": payeeID1})
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", status, env.raw)
	}
}

func TestSortPayeeList_ByName(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	createPayee(t, h, token, payeeID1, "Zebra")
	createPayee(t, h, token, payeeID2, "Apple")
	createPayee(t, h, token, payeeID3, "Mango")

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/sort-payee-list", token,
		map[string]any{"by": "name", "direction": "asc"})
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	names := itemNames(t, env)
	want := []string{"Apple", "Mango", "Zebra"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestSortPayeeList_ByUsage(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	createPayee(t, h, token, payeeID1, "Alpha")
	createPayee(t, h, token, payeeID2, "Beta")
	// Gamma is the LAST alphabetical payee; seeding it as the most-used proves
	// usage-desc sorts by count (not by name).
	idGamma := createPayee(t, h, token, payeeID3, "Gamma")

	f := fixture.New(t, h.tdb)
	acctID := f.Account(fixture.Account{UserID: seedUserID, CurrencyID: fixture.USD, Name: "UsageAcct"})
	base := h.clock.t
	for _, offset := range []int{-3, -2, -1} {
		f.Transaction(fixture.Transaction{
			UserID: seedUserID, AccountID: acctID, PayeeID: idGamma,
			Type: 0, Amount: "1.00000000", SpentAt: base.AddDate(0, 0, offset),
		})
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/sort-payee-list", token,
		map[string]any{"by": "usage", "direction": "desc", "periodMonths": 6})
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	names := itemNames(t, env)
	if len(names) == 0 || names[0] != "Gamma" {
		t.Fatalf("usage-desc order = %v, want the most-used payee (Gamma) first", names)
	}
}

func TestSortPayeeList_Validation(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	cases := []struct {
		body    map[string]any
		key     string
		wantMsg string
	}{
		{map[string]any{"direction": "asc"}, "by", "This value should not be blank."},
		{map[string]any{"by": "color", "direction": "asc"}, "by", "The value you selected is not a valid choice."},
		{map[string]any{"by": "name"}, "direction", "This value should not be blank."},
		{map[string]any{"by": "usage", "direction": "asc"}, "periodMonths", "This value should be an integer between 1 and 6."},
		{map[string]any{"by": "usage", "direction": "asc", "periodMonths": 7}, "periodMonths", "This value should be an integer between 1 and 6."},
		{map[string]any{"by": "name", "direction": "asc", "periodMonths": 3}, "periodMonths", "periodMonths is only valid when by is usage."},
	}
	for _, tc := range cases {
		status, env := h.do(t, http.MethodPost, "/api/v1/payee/sort-payee-list", token, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("body %v: status = %d, want 400; body: %s", tc.body, status, env.raw)
		}
		msgs := env.errorsMap()[tc.key]
		if len(msgs) == 0 || msgs[0] != tc.wantMsg {
			t.Fatalf("body %v: errors[%s] = %v, want %q", tc.body, tc.key, msgs, tc.wantMsg)
		}
	}
}

// itemNames extracts data.items[].name in order.
func itemNames(t *testing.T, env envelope) []string {
	t.Helper()
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	out := make([]string, len(res.Items))
	for i, it := range res.Items {
		out[i] = it.Name
	}
	return out
}
