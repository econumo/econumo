package account_test

import (
	"net/http"
	"testing"
)

type folderItemWrapper struct {
	Item folderItem `json:"item"`
}
type folderItemsWrapper struct {
	Items []folderItem `json:"items"`
}

func TestCreateFolder_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": "Savings"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[folderItemWrapper](t, env.Data)
	if res.Item.Name != "Savings" {
		t.Fatalf("name = %q, want Savings", res.Item.Name)
	}
	if res.Item.IsVisible != 1 {
		t.Fatalf("isVisible = %d, want 1", res.Item.IsVisible)
	}
	// Seeded "Main" folder is at position 0, so the new one is at 1.
	if res.Item.Position != 1 {
		t.Fatalf("position = %d, want 1", res.Item.Position)
	}
}

func TestCreateFolder_DuplicateName_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// "Main" already exists (seeded).
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": "Main"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (duplicate); body: %s", status, env.raw)
	}
}

func TestUpdateFolder_RenamesAndPersists(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-folder", tok, map[string]any{
		"id": seedFolderID, "name": "Primary",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[folderItemWrapper](t, env.Data)
	if res.Item.Name != "Primary" {
		t.Fatalf("name = %q, want Primary", res.Item.Name)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-folder-list", tok, nil)
	list := mustUnmarshal[folderItemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].Name != "Primary" {
		t.Fatalf("list = %+v, want one folder Primary", list.Items)
	}
}

func TestHideShowFolder(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	if st, _ := h.do(t, http.MethodPost, "/api/v1/account/hide-folder", tok, map[string]any{"id": seedFolderID}); st != http.StatusOK {
		t.Fatalf("hide = %d, want 200", st)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-folder-list", tok, nil)
	list := mustUnmarshal[folderItemsWrapper](t, listEnv.Data)
	if list.Items[0].IsVisible != 0 {
		t.Fatalf("after hide isVisible = %d, want 0", list.Items[0].IsVisible)
	}

	if st, _ := h.do(t, http.MethodPost, "/api/v1/account/show-folder", tok, map[string]any{"id": seedFolderID}); st != http.StatusOK {
		t.Fatalf("show = %d, want 200", st)
	}
	_, listEnv2 := h.do(t, http.MethodGet, "/api/v1/account/get-folder-list", tok, nil)
	list2 := mustUnmarshal[folderItemsWrapper](t, listEnv2.Data)
	if list2.Items[0].IsVisible != 1 {
		t.Fatalf("after show isVisible = %d, want 1", list2.Items[0].IsVisible)
	}
}

func TestGetFolderList_OrderedByPosition(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": "Bills"}) // pos 1
	h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": "Cards"}) // pos 2

	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-folder-list", tok, nil)
	list := mustUnmarshal[folderItemsWrapper](t, env.Data)
	if len(list.Items) != 3 {
		t.Fatalf("got %d folders, want 3", len(list.Items))
	}
	// Main(0), Bills(1), Cards(2)
	if list.Items[0].Name != "Main" || list.Items[1].Name != "Bills" || list.Items[2].Name != "Cards" {
		t.Fatalf("order = [%s,%s,%s], want [Main,Bills,Cards]", list.Items[0].Name, list.Items[1].Name, list.Items[2].Name)
	}
}

func TestOrderFolderList_Reorders(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	_, bEnv := h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": "Bills"})
	bID := mustUnmarshal[folderItemWrapper](t, bEnv.Data).Item.ID

	status, env := h.do(t, http.MethodPost, "/api/v1/account/order-folder-list", tok, map[string]any{
		"changes": []map[string]any{
			{"id": bID, "position": 0},
			{"id": seedFolderID, "position": 1},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("order = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[folderItemsWrapper](t, env.Data)
	if res.Items[0].ID != bID || res.Items[1].ID != seedFolderID {
		t.Fatalf("order = [%s,%s], want [Bills, Main]", res.Items[0].ID, res.Items[1].ID)
	}
}

func TestOrderFolderList_Empty_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/account/order-folder-list", tok, map[string]any{"changes": []map[string]any{}})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestReplaceFolder_MovesAccountsAndDeletes(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// Second folder + an account in the seeded "Main" folder.
	_, bEnv := h.do(t, http.MethodPost, "/api/v1/account/create-folder", tok, map[string]any{"name": "Bills"})
	bID := mustUnmarshal[folderItemWrapper](t, bEnv.Data).Item.ID
	h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "0")) // lands in seedFolderID

	// Replace Main -> B: the account moves to B, Main is deleted.
	status, env := h.do(t, http.MethodPost, "/api/v1/account/replace-folder", tok, map[string]any{
		"id": seedFolderID, "replaceId": bID,
	})
	if status != http.StatusOK {
		t.Fatalf("replace = %d, want 200; body: %s", status, env.raw)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-folder-list", tok, nil)
	folders := mustUnmarshal[folderItemsWrapper](t, listEnv.Data)
	if len(folders.Items) != 1 || folders.Items[0].ID != bID {
		t.Fatalf("folders after replace = %+v, want only Bills", folders.Items)
	}
	// The account now reports folderId = B.
	_, accEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	accts := mustUnmarshal[accountItemsWrapper](t, accEnv.Data)
	if len(accts.Items) != 1 || accts.Items[0].FolderID == nil || *accts.Items[0].FolderID != bID {
		t.Fatalf("account folderId after replace = %v, want %s", accts.Items[0].FolderID, bID)
	}
}
