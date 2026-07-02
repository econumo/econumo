package apiparity

// The comprehensive API scenario catalogue. Each scenario is a sequence of HTTP
// calls (reads, and write->read sequences) replayed against the production
// handler; the enginecompare parity suite asserts every call's (status, raw
// body) is byte-identical across SQLite and PostgreSQL from an identical seed.
//
// Coverage target (confirmed): every read/list endpoint across all 9 modules,
// plus a create/update/delete-then-read sequence per mutating module so the
// persisted state after a write is compared too — catching engine differences in
// upserts, datetime writes, decimal rounding, and ordering that a read-only
// sweep would miss.

// ---- read-endpoint catalogue (one scenario per module) ----

func init() {
	register(Scenario{Name: "user_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-user-data", Method: "POST", Path: "/api/v1/user/get-user-data", Auth: "owner", Body: map[string]any{}},
			{Label: "get-option-list", Method: "POST", Path: "/api/v1/user/get-option-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "currency_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-currency-list", Method: "POST", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-currency-rate-list", Method: "POST", Path: "/api/v1/currency/get-currency-rate-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "category_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-category-list", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-category-list-guest", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "guest", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "tag_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-tag-list", Method: "POST", Path: "/api/v1/tag/get-tag-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "payee_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-payee-list", Method: "POST", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "account_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-account-list", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-account-list-guest", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "guest", Body: map[string]any{}},
			{Label: "get-folder-list", Method: "POST", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "transaction_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-transaction-list", Method: "POST", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "connection_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-connection-list", Method: "POST", Path: "/api/v1/connection/get-connection-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-connection-list-guest", Method: "POST", Path: "/api/v1/connection/get-connection-list", Auth: "guest", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "budget_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-budget-list", Method: "POST", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// ---- write -> read sequences (per mutating module) ----

	register(Scenario{Name: "category_write_read", Calls: func() []Call {
		const newCat = "c0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": newCat, "name": "Travel", "type": 0, "icon": "plane"}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-category", Method: "POST", Path: "/api/v1/category/update-category", Auth: "owner",
				Body: map[string]any{"id": newCat, "name": "Travel2", "icon": "plane2"}},
			{Label: "archive-category", Method: "POST", Path: "/api/v1/category/archive-category", Auth: "owner", Body: map[string]any{"id": newCat}},
			{Label: "read-after-archive", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "unarchive-category", Method: "POST", Path: "/api/v1/category/unarchive-category", Auth: "owner", Body: map[string]any{"id": newCat}},
			{Label: "delete-category", Method: "POST", Path: "/api/v1/category/delete-category", Auth: "owner", Body: map[string]any{"id": newCat}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "tag_write_read", Calls: func() []Call {
		const newTag = "10000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-tag", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "owner", Body: map[string]any{"id": newTag, "name": "Urgent"}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/tag/get-tag-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-tag", Method: "POST", Path: "/api/v1/tag/update-tag", Auth: "owner", Body: map[string]any{"id": newTag, "name": "Urgent2"}},
			{Label: "archive-tag", Method: "POST", Path: "/api/v1/tag/archive-tag", Auth: "owner", Body: map[string]any{"id": newTag}},
			{Label: "unarchive-tag", Method: "POST", Path: "/api/v1/tag/unarchive-tag", Auth: "owner", Body: map[string]any{"id": newTag}},
			{Label: "delete-tag", Method: "POST", Path: "/api/v1/tag/delete-tag", Auth: "owner", Body: map[string]any{"id": newTag}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/tag/get-tag-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "payee_write_read", Calls: func() []Call {
		const newPayee = "20000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-payee", Method: "POST", Path: "/api/v1/payee/create-payee", Auth: "owner", Body: map[string]any{"id": newPayee, "name": "Cafe"}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-payee", Method: "POST", Path: "/api/v1/payee/update-payee", Auth: "owner", Body: map[string]any{"id": newPayee, "name": "Cafe2"}},
			{Label: "archive-payee", Method: "POST", Path: "/api/v1/payee/archive-payee", Auth: "owner", Body: map[string]any{"id": newPayee}},
			{Label: "unarchive-payee", Method: "POST", Path: "/api/v1/payee/unarchive-payee", Auth: "owner", Body: map[string]any{"id": newPayee}},
			{Label: "delete-payee", Method: "POST", Path: "/api/v1/payee/delete-payee", Auth: "owner", Body: map[string]any{"id": newPayee}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "account_write_read", Calls: func() []Call {
		const newAcct = "a0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{"id": newAcct, "name": "Savings", "type": 2, "icon": "bank", "currencyId": USD}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-account", Method: "POST", Path: "/api/v1/account/update-account", Auth: "owner",
				Body: map[string]any{"id": newAcct, "name": "Savings2", "icon": "bank2"}},
			{Label: "read-after-update", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-account", Method: "POST", Path: "/api/v1/account/delete-account", Auth: "owner", Body: map[string]any{"id": newAcct}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "transaction_write_read", Calls: func() []Call {
		const newTxn = "d0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-transaction", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": newTxn, "accountId": OwnerAccount, "type": 1,
					"amount": "9.99", "categoryId": CatFood, "date": "2024-04-02 10:00:00",
				}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
			{Label: "account-list-after-create", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-transaction", Method: "POST", Path: "/api/v1/transaction/delete-transaction", Auth: "owner", Body: map[string]any{"id": newTxn}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// transaction_write_read_shared creates and deletes a transaction on an
	// account SHARED with the caller (the owner holds a user-role grant on the
	// guest-owned SharedAccount). Exercises the shared-account write-access path
	// through the real server.BuildAPI on both engines — the regression where the
	// Go port had reduced the check to owner-only and returned a 400 here.
	register(Scenario{Name: "transaction_write_read_shared", Calls: func() []Call {
		const newTxn = "d0000000-0000-0000-0000-0000000000fe"
		return []Call{
			{Label: "create-on-shared", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": newTxn, "accountId": SharedAccount, "type": 1,
					"amount": "7.25", "categoryId": CatFood, "date": "2024-04-03 10:00:00",
				}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
			{Label: "account-list-after-create", Method: "POST", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-on-shared", Method: "POST", Path: "/api/v1/transaction/delete-transaction", Auth: "owner", Body: map[string]any{"id": newTxn}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// create_for_shared_account_denied exercises the create-for-account access
	// path for category/tag/payee on SharedAccount, where the owner holds only a
	// USER grant (role 1) — insufficient for create-for-account, which requires
	// owner/admin. The denial runs the account-owner + grant-role lookups
	// (engine-specific SQL) and must be byte-identical across SQLite and
	// PostgreSQL.
	register(Scenario{Name: "create_for_shared_account_denied", Calls: func() []Call {
		return []Call{
			{Label: "create-category-shared", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000fd", "name": "Shared", "type": 0, "icon": "tag", "accountId": SharedAccount}},
			{Label: "create-tag-shared", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "owner",
				Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000fd", "name": "SharedTag", "accountId": SharedAccount}},
			{Label: "create-payee-shared", Method: "POST", Path: "/api/v1/payee/create-payee", Auth: "owner",
				Body: map[string]any{"id": "20000000-0000-0000-0000-0000000000fd", "name": "SharedPayee", "accountId": SharedAccount}},
		}
	}})

	register(Scenario{Name: "budget_write_read", Calls: func() []Call {
		const newBudget = "b0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": newBudget, "name": "Trip", "currencyId": USD, "startedAt": "2024-04-01"}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
			{Label: "set-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{
					"budgetId": newBudget, "elementId": CatFood, "elementType": 0,
					"period": "2024-04-01", "amount": "300", "currencyId": USD,
				}},
			{Label: "update-budget", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": newBudget, "name": "Trip2"}},
			{Label: "read-after-update", Method: "POST", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-budget", Method: "POST", Path: "/api/v1/budget/delete-budget", Auth: "owner", Body: map[string]any{"id": newBudget}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "user_write_read", Calls: func() []Call {
		return []Call{
			{Label: "update-name", Method: "POST", Path: "/api/v1/user/update-name", Auth: "owner", Body: map[string]any{"name": "Renamed"}},
			{Label: "update-report-period", Method: "POST", Path: "/api/v1/user/update-report-period", Auth: "owner", Body: map[string]any{"reportPeriod": "weekly"}},
			{Label: "read-after-update", Method: "POST", Path: "/api/v1/user/get-user-data", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "folder_write_read", Calls: func() []Call {
		const newFolder = "f0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-folder", Method: "POST", Path: "/api/v1/account/create-folder", Auth: "owner", Body: map[string]any{"id": newFolder, "name": "Trips"}},
			{Label: "read-after-create", Method: "POST", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-folder", Method: "POST", Path: "/api/v1/account/update-folder", Auth: "owner", Body: map[string]any{"id": newFolder, "name": "Trips2"}},
			{Label: "hide-folder", Method: "POST", Path: "/api/v1/account/hide-folder", Auth: "owner", Body: map[string]any{"id": newFolder, "isVisible": false}},
			{Label: "read-after-hide", Method: "POST", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-folder", Method: "POST", Path: "/api/v1/account/delete-folder", Auth: "owner", Body: map[string]any{"id": newFolder}},
			{Label: "read-after-delete", Method: "POST", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// ---- error-path parity (validation + auth envelopes must also match) ----

	register(Scenario{Name: "error_paths", Calls: func() []Call {
		return []Call{
			{Label: "unauthorized", Method: "POST", Path: "/api/v1/user/get-user-data", Auth: "", Body: map[string]any{}},
			{Label: "validation-empty-name", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000ee", "name": "", "type": 0, "icon": "x"}},
			{Label: "not-found-update", Method: "POST", Path: "/api/v1/category/update-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000dd", "name": "Ghost", "icon": "x"}},
		}
	}})
}
