package main

import (
	"fmt"
	"net/url"
)

// allCases returns every mutation case, in a safe order within each group
// (create before update before delete). The driver runs ONE per fresh DB copy,
// so inter-case ordering does not matter for isolation — the order is only for
// readability and for -list.
func allCases() []mutationCase {
	var cs []mutationCase
	cs = append(cs, categoryCases()...)
	cs = append(cs, tagCases()...)
	cs = append(cs, payeeCases()...)
	cs = append(cs, accountCases()...)
	cs = append(cs, folderCases()...)
	return cs
}

const acctList = "/api/v1/account/get-account-list"
const folderList = "/api/v1/account/get-folder-list"

// firstOwnedAccount returns the first account OWNED by the logged-in user. The
// account list embeds the owner as a nested object under "owner" (not the
// "ownerUserId" string the category-shaped resources use), so ownership is
// resolved via owner.id.
func firstOwnedAccount(php *client) (map[string]any, string, error) {
	me, err := php.userID()
	if err != nil {
		return nil, "", err
	}
	items, err := php.items(acctList, nil)
	if err != nil {
		return nil, "", err
	}
	for _, it := range items {
		owner, _ := it["owner"].(map[string]any)
		oid, _ := owner["id"].(string)
		if oid == me {
			return it, "", nil
		}
	}
	return nil, "no owned account in " + acctList, nil
}

// currencyIDFromAccounts returns a real currency id by reading the first
// account's nested currency object (guaranteed to be a valid, existing currency
// id so create-account's NotBlank+Uuid currencyId passes on both backends).
func currencyIDFromAccounts(php *client) (string, string, error) {
	items, err := php.items(acctList, nil)
	if err != nil {
		return "", "", err
	}
	for _, it := range items {
		cur, _ := it["currency"].(map[string]any)
		if id, _ := cur["id"].(string); id != "" {
			return id, "", nil
		}
	}
	return "", "no currency in account list", nil
}

// firstOwnedFolderID returns the id of the first folder (folders are all owned
// by the user — get-folder-list is per-user).
func firstOwnedFolderID(php *client) (string, string, error) {
	return firstID(php, folderList, nil)
}

func accountCases() []mutationCase {
	state := func(php *client) string { return acctList }
	return []mutationCase{
		{
			name: "account/create",
			build: func(php *client) (string, map[string]any, string, error) {
				currencyID, skip, err := currencyIDFromAccounts(php)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				folderID, skip, err := firstOwnedFolderID(php)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				id := newUUID()
				body := map[string]any{
					"id":         id, // operation id; PHP mints a fresh entity id
					"name":       "ZZ Compare " + id[:8],
					"currencyId": currencyID,
					"balance":    "123.45",
					"icon":       "wallet",
					"folderId":   folderID,
				}
				return "/api/v1/account/create-account", body, "", nil
			},
			stateRead: state,
			// The created account's id is a server-minted UUIDv7 (differs per
			// backend); the correction transaction it seeds is dated at the
			// account's createdAt (now, differs). Blank ids + now-timestamps.
			volatile: []string{"id", "createdAt", "updatedAt"},
		},
		{
			name: "account/update",
			build: func(php *client) (string, map[string]any, string, error) {
				acct, skip, err := firstOwnedAccount(php)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				id, _ := acct["id"].(string)
				body := map[string]any{
					"id":        id,
					"name":      "ZZ Renamed " + id[:8],
					"balance":   "98765.43", // differs from seed -> forces a correction tx
					"icon":      "star",
					"updatedAt": "2024-01-01 12:00:00",
				}
				return "/api/v1/account/update-account", body, "", nil
			},
			stateRead: state,
			// The correction transaction's id is server-minted; the account's
			// updatedAt is bumped to now. Both differ between backends.
			volatile: []string{"id", "updatedAt", "createdAt"},
		},
		{
			name: "account/delete",
			build: func(php *client) (string, map[string]any, string, error) {
				acct, skip, err := firstOwnedAccount(php)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				id, _ := acct["id"].(string)
				return "/api/v1/account/delete-account", map[string]any{"id": id}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: "account/order",
			build: func(php *client) (string, map[string]any, string, error) {
				me, err := php.userID()
				if err != nil {
					return "", nil, "", err
				}
				items, err := php.items(acctList, nil)
				if err != nil {
					return "", nil, "", err
				}
				// Collect own accounts that have a folderId (the SPA always sends
				// folderId in each change; the form requires it NotBlank+Uuid).
				var owned []map[string]any
				for _, it := range items {
					owner, _ := it["owner"].(map[string]any)
					if oid, _ := owner["id"].(string); oid != me {
						continue
					}
					if fid, _ := it["folderId"].(string); fid != "" {
						owned = append(owned, it)
					}
				}
				if len(owned) < 2 {
					return "", nil, "need >=2 owned accounts with a folder to reorder", nil
				}
				a, b := owned[0], owned[1]
				aID, _ := a["id"].(string)
				bID, _ := b["id"].(string)
				aFolder, _ := a["folderId"].(string)
				bFolder, _ := b["folderId"].(string)
				// Swap positions, keep each account in its current folder.
				changes := []map[string]any{
					{"id": aID, "position": 1, "folderId": aFolder},
					{"id": bID, "position": 0, "folderId": bFolder},
				}
				return "/api/v1/account/order-account-list", map[string]any{"changes": changes}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
	}
}

func folderCases() []mutationCase {
	state := func(php *client) string { return folderList }
	return []mutationCase{
		{
			name: "folder/create",
			build: func(php *client) (string, map[string]any, string, error) {
				// create-folder takes {name} ONLY (PHP mints the id).
				body := map[string]any{"name": "ZZ Folder " + newUUID()[:8]}
				return "/api/v1/account/create-folder", body, "", nil
			},
			stateRead: state,
			// The created folder's id is server-minted (differs per backend).
			volatile: []string{"id"},
		},
		{
			name: "folder/update",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := firstOwnedFolderID(php)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				body := map[string]any{"id": id, "name": "ZZ Renamed " + id[:8]}
				return "/api/v1/account/update-folder", body, "", nil
			},
			stateRead: state,
		},
		{
			name: "folder/hide",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := firstVisibleFolderID(php, false)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/account/hide-folder", map[string]any{"id": id}, "", nil
			},
			stateRead: state,
		},
		{
			name: "folder/show",
			build: func(php *client) (string, map[string]any, string, error) {
				// Pick a hidden folder if any; else hide-then-show is not testable
				// purely (no hidden seed). Fall back to any folder (show is
				// idempotent on a visible folder -> no-op, identical on both).
				id, skip, err := firstVisibleFolderID(php, true)
				if skip != "" {
					id, skip, err = firstOwnedFolderID(php)
				}
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/account/show-folder", map[string]any{"id": id}, "", nil
			},
			stateRead: state,
		},
		{
			name: "folder/replace",
			build: func(php *client) (string, map[string]any, string, error) {
				items, err := php.items(folderList, nil)
				if err != nil {
					return "", nil, "", err
				}
				if len(items) < 2 {
					return "", nil, "need >=2 folders to replace", nil
				}
				id, _ := items[0]["id"].(string)
				replaceID, _ := items[1]["id"].(string)
				return "/api/v1/account/replace-folder", map[string]any{"id": id, "replaceId": replaceID}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: "folder/order",
			build: func(php *client) (string, map[string]any, string, error) {
				items, err := php.items(folderList, nil)
				if err != nil {
					return "", nil, "", err
				}
				if len(items) < 2 {
					return "", nil, "need >=2 folders to reorder", nil
				}
				a, _ := items[0]["id"].(string)
				b, _ := items[1]["id"].(string)
				aPos := intOf(items[0]["position"])
				bPos := intOf(items[1]["position"])
				// Swap the first two folders' positions.
				changes := []map[string]any{
					{"id": a, "position": bPos},
					{"id": b, "position": aPos},
				}
				return "/api/v1/account/order-folder-list", map[string]any{"changes": changes}, "", nil
			},
			stateRead: state,
		},
	}
}

// firstVisibleFolderID returns the id of the first folder whose isVisible flag
// matches want (PHP serializes isVisible as int 0/1).
func firstVisibleFolderID(php *client, hidden bool) (string, string, error) {
	items, err := php.items(folderList, nil)
	if err != nil {
		return "", "", err
	}
	for _, it := range items {
		vis := intOf(it["isVisible"]) != 0
		if vis == !hidden {
			if id, _ := it["id"].(string); id != "" {
				return id, "", nil
			}
		}
	}
	if hidden {
		return "", "no hidden folder", nil
	}
	return "", "no visible folder", nil
}

// intOf coerces a JSON number/string to int.
func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return 0
}

// simpleNamedCases builds the create/update/archive/unarchive/delete/order cases
// for the category-shaped resources (category, tag, payee). They share the same
// request shapes; only the route prefix, list endpoint, and the create "type"
// field (categories only) differ. withType adds {"type":"expense"} to create.
func simpleNamedCases(resource, listPath string, withType bool) []mutationCase {
	state := func(php *client) string { return listPath }
	base := "/api/v1/" + resource + "/"
	// Only the category form has an icon field; tag/payee forms reject unknown
	// fields, so don't send icon for them (the real SPA doesn't either).
	withIcon := withType
	return []mutationCase{
		{
			name: resource + "/create",
			build: func(php *client) (string, map[string]any, string, error) {
				id := newUUID()
				body := map[string]any{"id": id, "name": "ZZ Compare " + id[:8]}
				if withType {
					body["type"] = "expense"
				}
				if withIcon {
					body["icon"] = "label"
				}
				return base + "create-" + resource, body, "", nil
			},
			stateRead: state,
			volatile:  []string{"id", "createdAt", "updatedAt"},
		},
		{
			name: resource + "/update",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := firstOwnedID(php, listPath, nil)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				body := map[string]any{"id": id, "name": "ZZ Renamed " + id[:8]}
				if withIcon {
					body["icon"] = "star"
				}
				return base + "update-" + resource, body, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: resource + "/archive",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := pickByArchived(php, listPath, nil, false)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return base + "archive-" + resource, map[string]any{"id": id}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: resource + "/unarchive",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := pickByArchived(php, listPath, nil, true)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return base + "unarchive-" + resource, map[string]any{"id": id}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: resource + "/delete",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := firstOwnedID(php, listPath, nil)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				body := map[string]any{"id": id}
				if withType { // only category's delete takes a mode (delete/replace)
					body["mode"] = "delete"
				}
				return base + "delete-" + resource, body, "", nil
			},
			stateRead: state,
		},
		{
			name: resource + "/order",
			build: func(php *client) (string, map[string]any, string, error) {
				items, err := php.items(listPath, nil)
				if err != nil {
					return "", nil, "", err
				}
				if len(items) < 2 {
					return "", nil, "need >=2 to reorder", nil
				}
				a, _ := items[0]["id"].(string)
				b, _ := items[1]["id"].(string)
				changes := []map[string]any{{"id": a, "position": 1}, {"id": b, "position": 0}}
				return base + "order-" + resource + "-list", map[string]any{"changes": changes}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
	}
}

func tagCases() []mutationCase {
	return simpleNamedCases("tag", "/api/v1/tag/get-tag-list", false)
}

func payeeCases() []mutationCase {
	return simpleNamedCases("payee", "/api/v1/payee/get-payee-list", false)
}

// firstID returns the id of the first item from a list read, or skip if empty.
func firstID(php *client, path string, q url.Values) (string, string, error) {
	items, err := php.items(path, q)
	if err != nil {
		return "", "", err
	}
	if len(items) == 0 {
		return "", "no items in " + path, nil
	}
	id, _ := items[0]["id"].(string)
	if id == "" {
		return "", "first item has no id", nil
	}
	return id, "", nil
}

// firstOwnedID returns the id of the first item OWNED by the logged-in user
// (ownerUserId == me). Mutations like update/archive/delete require ownership;
// list reads also include shared items the user can't mutate, so we must filter.
func firstOwnedID(php *client, path string, q url.Values) (string, string, error) {
	me, err := php.userID()
	if err != nil {
		return "", "", err
	}
	items, err := php.items(path, q)
	if err != nil {
		return "", "", err
	}
	for _, it := range items {
		owner, _ := it["ownerUserId"].(string)
		if owner == "" || owner == me {
			if id, _ := it["id"].(string); id != "" {
				return id, "", nil
			}
		}
	}
	return "", "no owned item in " + path, nil
}

// pickByArchived returns the id of the first item whose isArchived flag matches
// want (PHP serializes isArchived as 0/1). Used to pick a valid archive target.
func pickByArchived(php *client, path string, q url.Values, want bool) (string, string, error) {
	me, err := php.userID()
	if err != nil {
		return "", "", err
	}
	items, err := php.items(path, q)
	if err != nil {
		return "", "", err
	}
	for _, it := range items {
		owner, _ := it["ownerUserId"].(string)
		if owner != "" && owner != me {
			continue // can only archive own items
		}
		arch := false
		switch v := it["isArchived"].(type) {
		case bool:
			arch = v
		case float64:
			arch = v != 0
		}
		if arch == want {
			if id, _ := it["id"].(string); id != "" {
				return id, "", nil
			}
		}
	}
	return "", fmt.Sprintf("no owned item with isArchived=%v in %s", want, path), nil
}

const catList = "/api/v1/category/get-category-list"

func categoryCases() []mutationCase {
	state := func(php *client) string { return catList }
	return []mutationCase{
		{
			name: "category/create",
			build: func(php *client) (string, map[string]any, string, error) {
				id := newUUID()
				return "/api/v1/category/create-category", map[string]any{
					"id":   id,
					"name": "ZZ Compare " + id[:8],
					"type": "expense",
					"icon": "label",
				}, "", nil
			},
			stateRead: state,
			// The created category's id is a fresh server-minted UUIDv7 (differs per
			// backend) and created/updated = now; blank them so only the persisted
			// business fields (name/type/icon/position/owner) are compared.
			volatile: []string{"id", "createdAt", "updatedAt"},
		},
		{
			name: "category/update",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := firstOwnedID(php, catList, nil)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/category/update-category", map[string]any{
					"id":   id,
					"name": "ZZ Renamed " + id[:8],
					"icon": "star",
				}, "", nil
			},
			stateRead: state,
			// updatedAt is bumped to now by both backends; the two POSTs land at
			// slightly different instants, so blank it.
			volatile: []string{"updatedAt"},
		},
		{
			name: "category/archive",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := pickByArchived(php, catList, nil, false)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/category/archive-category", map[string]any{"id": id}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: "category/unarchive",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := pickByArchived(php, catList, nil, true)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/category/unarchive-category", map[string]any{"id": id}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
		{
			name: "category/delete",
			build: func(php *client) (string, map[string]any, string, error) {
				id, skip, err := firstOwnedID(php, catList, nil)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/category/delete-category", map[string]any{
					"id":   id,
					"mode": "delete",
				}, "", nil
			},
			stateRead: state,
		},
		{
			name: "category/order",
			build: func(php *client) (string, map[string]any, string, error) {
				items, err := php.items(catList, nil)
				if err != nil {
					return "", nil, "", err
				}
				if len(items) < 2 {
					return "", nil, "need >=2 categories to reorder", nil
				}
				// Swap the positions of the first two own categories.
				a, _ := items[0]["id"].(string)
				b, _ := items[1]["id"].(string)
				// The frontend sends position as a JSON number ({id, position:number}).
				changes := []map[string]any{
					{"id": a, "position": 1},
					{"id": b, "position": 0},
				}
				return "/api/v1/category/order-category-list", map[string]any{"changes": changes}, "", nil
			},
			stateRead: state,
			volatile:  []string{"updatedAt"},
		},
	}
}
