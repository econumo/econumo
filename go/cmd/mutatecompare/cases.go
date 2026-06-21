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
	return cs
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
