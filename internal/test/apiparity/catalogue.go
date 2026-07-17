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
			{Label: "get-user-data", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner", Body: map[string]any{}},
			{Label: "get-option-list", Method: "GET", Path: "/api/v1/user/get-option-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "currency_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-currency-list", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-currency-rate-list", Method: "GET", Path: "/api/v1/currency/get-currency-rate-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "category_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-category-list", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-category-list-guest", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "guest", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "tag_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-tag-list", Method: "GET", Path: "/api/v1/tag/get-tag-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "payee_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-payee-list", Method: "GET", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "account_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-account-list", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-account-list-guest", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "guest", Body: map[string]any{}},
			{Label: "get-folder-list", Method: "GET", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "transaction_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-transaction-list", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "connection_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-connection-list", Method: "GET", Path: "/api/v1/connection/get-connection-list", Auth: "owner", Body: map[string]any{}},
			{Label: "get-connection-list-guest", Method: "GET", Path: "/api/v1/connection/get-connection-list", Auth: "guest", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "budget_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-budget-list", Method: "GET", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// ---- write -> read sequences (per mutating module) ----

	register(Scenario{Name: "category_write_read", Calls: func() []Call {
		const newCat = "c0000000-0000-0000-0000-0000000000ff"
		var catID string
		return []Call{
			{Label: "create-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": newCat, "name": "Travel", "type": "expense", "icon": "plane"}, CaptureIDInto: &catID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-category", Method: "POST", Path: "/api/v1/category/update-category", Auth: "owner",
				Body: map[string]any{"id": &catID, "name": "Travel2", "icon": "plane2"}},
			{Label: "archive-category", Method: "POST", Path: "/api/v1/category/archive-category", Auth: "owner", Body: map[string]any{"id": &catID}},
			{Label: "read-after-archive", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "unarchive-category", Method: "POST", Path: "/api/v1/category/unarchive-category", Auth: "owner", Body: map[string]any{"id": &catID}},
			{Label: "delete-category", Method: "POST", Path: "/api/v1/category/delete-category", Auth: "owner", Body: map[string]any{"id": &catID, "mode": "delete"}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "tag_write_read", Calls: func() []Call {
		const newTag = "10000000-0000-0000-0000-0000000000ff"
		var tagID string
		return []Call{
			{Label: "create-tag", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "owner", Body: map[string]any{"id": newTag, "name": "Urgent"}, CaptureIDInto: &tagID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/tag/get-tag-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-tag", Method: "POST", Path: "/api/v1/tag/update-tag", Auth: "owner", Body: map[string]any{"id": &tagID, "name": "Urgent2"}},
			{Label: "archive-tag", Method: "POST", Path: "/api/v1/tag/archive-tag", Auth: "owner", Body: map[string]any{"id": &tagID}},
			{Label: "unarchive-tag", Method: "POST", Path: "/api/v1/tag/unarchive-tag", Auth: "owner", Body: map[string]any{"id": &tagID}},
			{Label: "delete-tag", Method: "POST", Path: "/api/v1/tag/delete-tag", Auth: "owner", Body: map[string]any{"id": &tagID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/tag/get-tag-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "payee_write_read", Calls: func() []Call {
		const newPayee = "20000000-0000-0000-0000-0000000000ff"
		var payeeID string
		return []Call{
			{Label: "create-payee", Method: "POST", Path: "/api/v1/payee/create-payee", Auth: "owner", Body: map[string]any{"id": newPayee, "name": "Cafe"}, CaptureIDInto: &payeeID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-payee", Method: "POST", Path: "/api/v1/payee/update-payee", Auth: "owner", Body: map[string]any{"id": &payeeID, "name": "Cafe2"}},
			{Label: "archive-payee", Method: "POST", Path: "/api/v1/payee/archive-payee", Auth: "owner", Body: map[string]any{"id": &payeeID}},
			{Label: "unarchive-payee", Method: "POST", Path: "/api/v1/payee/unarchive-payee", Auth: "owner", Body: map[string]any{"id": &payeeID}},
			{Label: "delete-payee", Method: "POST", Path: "/api/v1/payee/delete-payee", Auth: "owner", Body: map[string]any{"id": &payeeID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// The fixture's only global currency is USD (seeded by the baseline
	// migration), which is ALSO both the instance base currency
	// (harness CurrencyBase="USD") and the owner's profile currency
	// (f.DefaultOptions seeds currency=USD) — so hiding it is rejected by the
	// base-currency guard before the profile-currency guard even runs. There is
	// no second global currency to hide/show as a happy path without adding
	// DB-level fixture setup shared by every other scenario, so hide-currency's
	// happy path is left uncovered here and its only coverage is the frozen
	// base-currency-guard contract below; show-currency has no such guard (it's
	// an idempotent no-op on a currency that was never hidden) so it still gets
	// a genuine happy-path call.
	register(Scenario{Name: "currency_write_read", Calls: func() []Call {
		const opCreate = "cc000000-0000-0000-0000-0000000000f1"
		const opCreate2 = "cc000000-0000-0000-0000-0000000000f2"
		var curID string
		return []Call{
			{Label: "create-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
				Body: map[string]any{"id": opCreate, "code": "PTS", "name": "Points", "symbol": "pts", "fractionDigits": 0, "rate": "100"}, CaptureIDInto: &curID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
			{Label: "rates-after-create", Method: "GET", Path: "/api/v1/currency/get-currency-rate-list", Auth: "owner", Body: map[string]any{}},
			{Label: "err:create-duplicate-code", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
				Body: map[string]any{"id": opCreate2, "code": "PTS", "name": "Points again"}},
			{Label: "update-currency", Method: "POST", Path: "/api/v1/currency/update-currency", Auth: "owner",
				Body: map[string]any{"id": &curID, "name": "Kid points", "symbol": "kp", "fractionDigits": 2}},
			{Label: "err:update-foreign", Method: "POST", Path: "/api/v1/currency/update-currency", Auth: "guest",
				Body: map[string]any{"id": &curID, "name": "Hijack", "symbol": "x", "fractionDigits": 2}},
			{Label: "set-currency-rate", Method: "POST", Path: "/api/v1/currency/set-currency-rate", Auth: "owner",
				Body: map[string]any{"currencyId": &curID, "rate": "120.5", "date": "2026-01-15"}},
			{Label: "err:set-rate-global", Method: "POST", Path: "/api/v1/currency/set-currency-rate", Auth: "owner",
				Body: map[string]any{"currencyId": USD, "rate": "2"}},
			{Label: "archive-currency", Method: "POST", Path: "/api/v1/currency/archive-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
			{Label: "read-after-archive", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
			{Label: "unarchive-currency", Method: "POST", Path: "/api/v1/currency/unarchive-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
			{Label: "err:hide-currency-base", Method: "POST", Path: "/api/v1/currency/hide-currency", Auth: "owner", Body: map[string]any{"id": USD}},
			{Label: "show-currency", Method: "POST", Path: "/api/v1/currency/show-currency", Auth: "owner", Body: map[string]any{"id": USD}},
			{Label: "err:hide-custom", Method: "POST", Path: "/api/v1/currency/hide-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
			{Label: "delete-currency", Method: "POST", Path: "/api/v1/currency/delete-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "account_write_read", Calls: func() []Call {
		const newAcct = "a0000000-0000-0000-0000-0000000000ff"
		var acctID string
		return []Call{
			{Label: "create-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{"id": newAcct, "name": "Savings", "icon": "bank", "currencyId": USD, "folderId": OwnerFolder}, CaptureIDInto: &acctID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-account", Method: "POST", Path: "/api/v1/account/update-account", Auth: "owner",
				Body: map[string]any{"id": &acctID, "name": "Savings2", "icon": "bank2", "updatedAt": "2024-01-01 00:00:00"}},
			{Label: "read-after-update", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-account", Method: "POST", Path: "/api/v1/account/delete-account", Auth: "owner", Body: map[string]any{"id": &acctID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "transaction_write_read", Calls: func() []Call {
		const newTxn = "d0000000-0000-0000-0000-0000000000ff"
		var txnID string
		return []Call{
			{Label: "create-transaction", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": newTxn, "accountId": OwnerAccount, "type": "expense",
					"amount": "9.99", "categoryId": CatFood, "date": "2024-04-02 10:00:00",
				}, CaptureIDInto: &txnID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
			{Label: "account-list-after-create", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-transaction", Method: "POST", Path: "/api/v1/transaction/delete-transaction", Auth: "owner", Body: map[string]any{"id": &txnID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// transaction_write_read_shared creates and deletes a transaction on an
	// account SHARED with the caller (the owner holds a user-role grant on the
	// guest-owned SharedAccount). Exercises the shared-account write-access path
	// through the real server.BuildAPI on both engines — the regression where the
	// Go port had reduced the check to owner-only and returned a 400 here.
	register(Scenario{Name: "transaction_write_read_shared", Calls: func() []Call {
		const newTxn = "d0000000-0000-0000-0000-0000000000fe"
		var txnID string
		return []Call{
			{Label: "create-on-shared", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": newTxn, "accountId": SharedAccount, "type": "expense",
					"amount": "7.25", "categoryId": CatFood, "date": "2024-04-03 10:00:00",
				}, CaptureIDInto: &txnID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
			{Label: "account-list-after-create", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-on-shared", Method: "POST", Path: "/api/v1/transaction/delete-transaction", Auth: "owner", Body: map[string]any{"id": &txnID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner", Body: map[string]any{}},
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
			{Label: "err:create-category-shared", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000fd", "name": "Shared", "type": "expense", "icon": "tag", "accountId": SharedAccount}},
			{Label: "err:create-tag-shared", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "owner",
				Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000fd", "name": "SharedTag", "accountId": SharedAccount}},
			{Label: "err:create-payee-shared", Method: "POST", Path: "/api/v1/payee/create-payee", Auth: "owner",
				Body: map[string]any{"id": "20000000-0000-0000-0000-0000000000fd", "name": "SharedPayee", "accountId": SharedAccount}},
		}
	}})

	register(Scenario{Name: "budget_write_read", Calls: func() []Call {
		const newBudget = "b0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": newBudget, "name": "Trip", "currencyId": USD, "startDate": "2024-04-01"}},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
			{Label: "set-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{
					"budgetId": newBudget, "elementId": CatFood,
					"period": "2024-04-01", "amount": "300",
				}},
			{Label: "update-budget", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": newBudget, "name": "Trip2", "currencyId": USD}},
			{Label: "read-after-update", Method: "GET", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
			{Label: "delete-budget", Method: "POST", Path: "/api/v1/budget/delete-budget", Auth: "owner", Body: map[string]any{"id": newBudget}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/budget/get-budget-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	register(Scenario{Name: "user_write_read", Calls: func() []Call {
		return []Call{
			{Label: "update-name", Method: "POST", Path: "/api/v1/user/update-name", Auth: "owner", Body: map[string]any{"name": "Renamed"}},
			{Label: "update-report-period", Method: "POST", Path: "/api/v1/user/update-report-period", Auth: "owner", Body: map[string]any{"value": "monthly"}},
			{Label: "update-language", Method: "POST", Path: "/api/v1/user/update-language", Auth: "owner", Body: map[string]any{"language": "ru"}},
			{Label: "read-after-update", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// The account module has no delete-folder route (only budget folders can be
	// deleted); a folder's terminal lifecycle op is hide/show, both exercised
	// here.
	register(Scenario{Name: "folder_write_read", Calls: func() []Call {
		var folderID string
		return []Call{
			{Label: "create-folder", Method: "POST", Path: "/api/v1/account/create-folder", Auth: "owner", Body: map[string]any{"name": "Trips"}, CaptureIDInto: &folderID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
			{Label: "update-folder", Method: "POST", Path: "/api/v1/account/update-folder", Auth: "owner", Body: map[string]any{"id": &folderID, "name": "Trips2"}},
			{Label: "hide-folder", Method: "POST", Path: "/api/v1/account/hide-folder", Auth: "owner", Body: map[string]any{"id": &folderID}},
			{Label: "read-after-hide", Method: "GET", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
			{Label: "show-folder", Method: "POST", Path: "/api/v1/account/show-folder", Auth: "owner", Body: map[string]any{"id": &folderID}},
			{Label: "read-after-show", Method: "GET", Path: "/api/v1/account/get-folder-list", Auth: "owner", Body: map[string]any{}},
		}
	}})

	// ---- error-path parity (validation + auth envelopes must also match) ----

	register(Scenario{Name: "error_paths", Calls: func() []Call {
		return []Call{
			{Label: "err:unauthorized", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "", Body: map[string]any{}},
			{Label: "err:validation-empty-name", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000ee", "name": "", "type": "expense", "icon": "x"}},
			{Label: "err:not-found-update", Method: "POST", Path: "/api/v1/category/update-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000dd", "name": "Ghost", "icon": "x"}},
		}
	}})
}
