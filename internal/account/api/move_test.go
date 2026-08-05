package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

type itemsEnvelope struct {
	Items []struct {
		ID       string  `json:"id"`
		Position int     `json:"position"`
		FolderId *string `json:"folderId"`
	} `json:"items"`
}

func moveIDs(t *testing.T, env envelope) []string {
	t.Helper()
	w := mustUnmarshal[itemsEnvelope](t, env.Data)
	out := make([]string, 0, len(w.Items))
	for _, it := range w.Items {
		out = append(out, it.ID)
	}
	return out
}

func assertMoveOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// createFolders makes n folders and returns their ids in creation order, which
// is also their initial order (creates append).
func createFolders(t *testing.T, h *harness, tok string, names ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(names))
	for _, n := range names {
		st, env := h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": n})
		if st != http.StatusOK {
			t.Fatalf("create-folder %q = %d; body: %s", n, st, env.raw)
		}
		ids = append(ids, mustUnmarshal[struct {
			Item folderItem `json:"item"`
		}](t, env.Data).Item.ID)
	}
	return ids
}

func TestMoveFolder_ToFront(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ids := createFolders(t, h, tok, "Alpha", "Bravo", "Charlie")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/move-folder", tok,
		map[string]any{"id": ids[2], "afterId": nil})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	got := moveIDs(t, env)
	// The default folder created with the harness may lead; assert the moved
	// folder now precedes the two it was created after.
	idx := map[string]int{}
	for i, id := range got {
		idx[id] = i
	}
	if idx[ids[2]] > idx[ids[0]] || idx[ids[2]] > idx[ids[1]] {
		t.Fatalf("moved folder did not reach the front: order=%v", got)
	}
	w := mustUnmarshal[itemsEnvelope](t, env.Data)
	for i, it := range w.Items {
		if it.Position != i {
			t.Fatalf("item %d position = %d, want dense index %d", i, it.Position, i)
		}
	}
}

func TestMoveFolder_AfterAnchor(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ids := createFolders(t, h, tok, "Alpha", "Bravo", "Charlie")

	_, env := h.do(t, http.MethodPost, "/api/v1/account/move-folder", tok,
		map[string]any{"id": ids[0], "afterId": ids[2]})
	got := moveIDs(t, env)
	idx := map[string]int{}
	for i, id := range got {
		idx[id] = i
	}
	if idx[ids[0]] != idx[ids[2]]+1 {
		t.Fatalf("folder Alpha is not directly after Charlie: order=%v", got)
	}
}

// TestMoveFolder_WritesExactlyOneRow is the point of the change: the endpoint
// this replaces rewrote every folder whose index shifted.
func TestMoveFolder_WritesExactlyOneRow(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ids := createFolders(t, h, tok, "Alpha", "Bravo", "Charlie")

	before := h.folderSortKeys(t)
	h.do(t, http.MethodPost, "/api/v1/account/move-folder", tok,
		map[string]any{"id": ids[2], "afterId": nil})
	after := h.folderSortKeys(t)

	changed := 0
	for id, k := range after {
		if before[id] != k {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("%d folders changed sort_key, want exactly 1", changed)
	}
}

func TestMoveFolder_MalformedID_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/account/move-folder", tok,
		map[string]any{"id": "not-a-uuid", "afterId": nil})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestMoveAccount_SetsFolderAndOrder(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	folders := createFolders(t, h, tok, "Target")
	acctID, _ := h.createAccount(t, "aaaa1111-0000-7000-8000-0000000000c1", "Cash", "0")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/move-account", tok,
		map[string]any{"id": acctID, "afterId": nil, "folderId": folders[0]})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	w := mustUnmarshal[itemsEnvelope](t, env.Data)
	for _, it := range w.Items {
		if it.ID != acctID {
			continue
		}
		if it.FolderId == nil || *it.FolderId != folders[0] {
			t.Fatalf("folderId = %v, want %q", it.FolderId, folders[0])
		}
		return
	}
	t.Fatalf("account %s missing from the response", acctID)
}

// TestMoveAccount_NullFolderRemovesFromEveryFolder pins the nullable folderId.
func TestMoveAccount_NullFolderRemovesFromEveryFolder(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	folders := createFolders(t, h, tok, "Target")
	acctID, _ := h.createAccount(t, "aaaa1111-0000-7000-8000-0000000000c1", "Cash", "0")

	h.do(t, http.MethodPost, "/api/v1/account/move-account", tok,
		map[string]any{"id": acctID, "afterId": nil, "folderId": folders[0]})
	_, env := h.do(t, http.MethodPost, "/api/v1/account/move-account", tok,
		map[string]any{"id": acctID, "afterId": nil, "folderId": nil})

	for _, it := range mustUnmarshal[itemsEnvelope](t, env.Data).Items {
		if it.ID == acctID && it.FolderId != nil {
			t.Fatalf("folderId = %q, want null after moving out of every folder", *it.FolderId)
		}
	}
}

func TestMoveAccount_UnknownAccountIsIgnored(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.createAccount(t, "aaaa1111-0000-7000-8000-0000000000c1", "Cash", "0")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/move-account", tok,
		map[string]any{"id": "00000000-0000-0000-0000-0000000000ff", "afterId": nil, "folderId": nil})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (an account the user cannot see is ignored); body: %s", status, env.raw)
	}
}

// TestMoveAccount_AnchorOnKeylessAccount covers legacy data: an (account, user)
// pair with no accounts_options row carries no sort key. Empty keys all compare
// equal and sort first, so without normalization no real key can land BETWEEN
// two keyless accounts -- anchoring on the first would drop the moved account
// after every keyless sibling instead of directly after its anchor.
func TestMoveAccount_AnchorOnKeylessAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// Seed three accounts directly, bypassing create-account, so NONE has an
	// accounts_options row -- the state migrated legacy data can be in.
	idA := "aaaa1111-0000-7000-8000-00000000000a"
	idB := "aaaa1111-0000-7000-8000-00000000000b"
	idC := "aaaa1111-0000-7000-8000-00000000000c"
	for _, a := range []struct{ id, name string }{{idA, "Alpha"}, {idB, "Bravo"}, {idC, "Charlie"}} {
		h.f.Account(fixture.Account{ID: a.id, UserID: seedUserID, CurrencyID: usdID, Name: a.name, Type: 2, Icon: "wallet"})
	}

	// Keyless accounts order by id, so the list reads A, B, C. Drop C after A.
	status, env := h.do(t, http.MethodPost, "/api/v1/account/move-account", tok,
		map[string]any{"id": idC, "afterId": idA, "folderId": nil})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	assertMoveOrder(t, moveIDs(t, env), []string{idA, idC, idB})
}
