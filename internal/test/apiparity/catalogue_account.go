package apiparity

func init() {
	register(Scenario{Name: "account_folder_writes", Calls: func() []Call {
		return []Call{
			{Label: "order-account-list", Method: "POST", Path: "/api/v1/account/order-account-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": OwnerAccount, "folderId": OwnerFolder, "position": 0}}}},
			{Label: "order-folder-list", Method: "POST", Path: "/api/v1/account/order-folder-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": OwnerFolder, "position": 0}, {"id": OwnerFolder2, "position": 1}}}},
			{Label: "show-folder", Method: "POST", Path: "/api/v1/account/show-folder", Auth: "owner",
				Body: map[string]any{"id": OwnerFolder2}},
			// Moves OwnerFolder2's accounts into OwnerFolder and DELETES OwnerFolder2.
			{Label: "replace-folder", Method: "POST", Path: "/api/v1/account/replace-folder", Auth: "owner",
				Body: map[string]any{"id": OwnerFolder2, "replaceId": OwnerFolder}},
			{Label: "get-account-list-after", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner"},
		}
	}})
}
