package apiparity

func init() {
	register(Scenario{Name: "account_folder_writes", Calls: func() []Call {
		return []Call{
			{Label: "move-account", Method: "POST", Path: "/api/v1/account/move-account", Auth: "owner",
				Body: map[string]any{"id": OwnerAccount, "afterId": nil, "folderId": OwnerFolder}},
			{Label: "move-folder", Method: "POST", Path: "/api/v1/account/move-folder", Auth: "owner",
				Body: map[string]any{"id": OwnerFolder2, "afterId": nil}},
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

	// Snapshot then restore: OwnerAccount holds categorized rows, so the
	// restore adds a "Balance adjustment" just before its earliest row created
	// since 2000; SharedAccount is shared to the owner without admin rights, so
	// the snapshot omits it and the restore is refused.
	register(Scenario{Name: "account_keep_balance", Calls: func() []Call {
		return []Call{
			{Label: "get-balance-snapshot", Method: "GET", Path: "/api/v1/account/get-balance-snapshot?accountIds=" + OwnerAccount + "," + SharedAccount, Auth: "owner"},
			{Label: "restore-balance", Method: "POST", Path: "/api/v1/account/restore-balance", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "balance": "2000", "openingCorrectionId": "", "since": "2000-01-01 00:00:00"}},
			{Label: "restore-balance-unchanged", Method: "POST", Path: "/api/v1/account/restore-balance", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "balance": "2000", "openingCorrectionId": "", "since": "2000-01-01 00:00:00"}},
			{Label: "get-account-transaction-list-after", Method: "GET", Path: "/api/v1/transaction/get-account-transaction-list?accountIds=" + OwnerAccount, Auth: "owner"},
			{Label: "err:restore-balance-no-admin", Method: "POST", Path: "/api/v1/account/restore-balance", Auth: "owner",
				Body: map[string]any{"accountId": SharedAccount, "balance": "10", "openingCorrectionId": "", "since": "2000-01-01 00:00:00"}},
			{Label: "err:restore-balance-invalid", Method: "POST", Path: "/api/v1/account/restore-balance", Auth: "owner",
				Body: map[string]any{"accountId": OwnerAccount, "balance": "ten", "openingCorrectionId": "", "since": "yesterday"}},
			{Label: "err:get-balance-snapshot-blank", Method: "GET", Path: "/api/v1/account/get-balance-snapshot?accountIds=", Auth: "owner"},
		}
	}})
}
