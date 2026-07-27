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
			// Pins the deletion: OwnerFolder2 ("Spare") must be gone from the list.
			{Label: "get-folder-list-after", Method: "GET", Path: "/api/v1/account/get-folder-list", Auth: "owner"},
		}
	}})

	// Grants access on the owner's account to the guest, walks the guest
	// through the pending -> accept -> (owner revoke) -> re-grant -> decline
	// handshake, and pins the 403 when accepting with nothing pending. Runs on
	// a fresh fixture DB, so it doesn't disturb the already-shared
	// SharedAccount or the other scenarios' assumptions.
	register(Scenario{Name: "account_access_writes", Calls: func() []Call {
		return []Call{
			{Label: "grant-access", Method: "POST", Path: "/api/v1/account/grant-access", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID, "role": "user"}},
			{Label: "get-account-list-pending", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "guest"},
			{Label: "accept-access", Method: "POST", Path: "/api/v1/account/accept-access", Auth: "guest",
				Body: map[string]any{"accountId": OwnerAccount, "folderId": GuestFolder}},
			{Label: "get-account-list-accepted", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "guest"},
			{Label: "revoke-access", Method: "POST", Path: "/api/v1/account/revoke-access", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID}},
			{Label: "grant-access-again", Method: "POST", Path: "/api/v1/account/grant-access", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "userId": GuestID, "role": "guest"}},
			{Label: "decline-access", Method: "POST", Path: "/api/v1/account/decline-access", Auth: "guest",
				Body: map[string]any{"accountId": OwnerAccount}},
			{Label: "err:accept-access-no-pending", Method: "POST", Path: "/api/v1/account/accept-access", Auth: "guest",
				Body: map[string]any{"accountId": OwnerAccount}},
		}
	}})
}
