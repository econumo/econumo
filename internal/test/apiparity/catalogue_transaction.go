package apiparity

// Transaction-module scenarios for update, CSV export, and multipart CSV
// import — the 3 routes carved out of missingFromCatalogue by this file.

import (
	"bytes"
	"mime/multipart"
)

func init() {
	register(Scenario{Name: "transaction_writes", Calls: func() []Call {
		return []Call{
			{Label: "update-transaction", Method: "POST", Path: "/api/v1/transaction/update-transaction", Auth: "owner",
				Body: map[string]any{
					"id": Txn1, "type": "expense", "amount": "15.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"date":        ClockTime.Format("2006-01-02 15:04:05"),
					"description": "lunch updated", "payeeId": PayeeShop,
				}},
			{Label: "export-transaction-list", Method: "GET", Path: "/api/v1/transaction/export-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
			// issue #261: a transfer that names no recipient must never be stored
			{Label: "err:create-transfer-without-recipient", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000e4", "type": "transfer", "amount": "300",
					"accountId": OwnerAccount, "date": ClockTime.Format("2006-01-02 15:04:05"),
				}},
		}
	}})

	// transaction_move: drag-and-drop on an account page. Txn1 and Txn2 share
	// the seed day with Txn2 on top; Txn1 goes to the top, then Txn2 moves to
	// the next day, after which Txn2 is no longer on the first day to drop under.
	register(Scenario{Name: "transaction_move", Calls: func() []Call {
		const path = "/api/v1/transaction/move-transaction"
		return []Call{
			{Label: "move-to-top", Method: "POST", Path: path, Auth: "owner",
				Body: map[string]any{"id": Txn1, "accountId": OwnerAccount, "date": "2024-04-01", "aboveId": nil}},
			{Label: "move-to-another-day", Method: "POST", Path: path, Auth: "owner",
				Body: map[string]any{"id": Txn2, "accountId": OwnerAccount, "date": "2024-04-02", "aboveId": nil}},
			{Label: "err:move-stale-day", Method: "POST", Path: path, Auth: "owner",
				Body: map[string]any{"id": Txn1, "accountId": OwnerAccount, "date": "2024-04-01", "aboveId": Txn2}},
			{Label: "err:move-blank", Method: "POST", Path: path, Auth: "owner", Body: map[string]any{}},
			{Label: "after-moves", Method: "GET", Path: "/api/v1/transaction/get-account-transaction-list?accountIds=" + OwnerAccount, Auth: "owner"},
		}
	}})

	// account_transaction_list: the per-account batch read the client cache
	// downloads from. A foreign id is dropped silently (never a 403); the
	// parameter being present-but-empty or malformed is a 400.
	register(Scenario{Name: "account_transaction_list", Calls: func() []Call {
		const path = "/api/v1/transaction/get-account-transaction-list"
		const unknownAccount = "a0000000-0000-0000-0000-0000000000ff"
		return []Call{
			{Label: "all-accessible", Method: "GET", Path: path, Auth: "owner"},
			{Label: "one-account", Method: "GET", Path: path + "?accountIds=" + OwnerAccount, Auth: "owner"},
			{Label: "owned-and-shared", Method: "GET", Path: path + "?accountIds=" + OwnerAccount + "," + SharedAccount, Auth: "owner"},
			{Label: "foreign-id-omitted", Method: "GET", Path: path + "?accountIds=" + OwnerAccount + "," + unknownAccount, Auth: "owner"},
			{Label: "other-users-account-only", Method: "GET", Path: path + "?accountIds=" + OwnerAccount, Auth: "guest"},
			{Label: "err:empty-ids", Method: "GET", Path: path + "?accountIds=", Auth: "owner"},
			{Label: "err:not-a-uuid", Method: "GET", Path: path + "?accountIds=nope", Auth: "owner"},
		}
	}})

	// transaction_labels covers a POPULATED labelIds (not just the ubiquitous
	// "[]" every other scenario shows): the create request lists LabelPersonal
	// before LabelWork (its id sorts AFTER LabelWork's), so a correct response
	// and a subsequent list read must both come back [LabelWork, LabelPersonal]
	// — exactly the cross-engine ORDER BY property enginecompare exists to
	// catch (Task 9 shipped a real divergence in this exact area).
	register(Scenario{Name: "transaction_labels", Calls: func() []Call {
		const newTxn = "d0000000-0000-0000-0000-0000000000fd"
		var txnID string
		return []Call{
			{Label: "create-with-labels", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": newTxn, "accountId": OwnerAccount, "type": "expense",
					"amount": "5.00", "categoryId": CatFood, "date": "2024-04-04 10:00:00",
					"labelIds": []string{LabelPersonal, LabelWork},
				}, CaptureIDInto: &txnID},
			{Label: "read-after-create", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
			{Label: "update-clears-labels", Method: "POST", Path: "/api/v1/transaction/update-transaction", Auth: "owner",
				Body: map[string]any{
					"id": &txnID, "accountId": OwnerAccount, "type": "expense",
					"amount": "5.00", "categoryId": CatFood, "date": "2024-04-04 10:00:00",
					"labelIds": []string{},
				}},
			{Label: "read-after-clear", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})

	// A deleted account's balance is pinned at zero (spec §5a), so no new flow
	// may be created on or into one: both sides of a create are rejected with
	// the coded transaction.account_deleted field error, whose wire shape this
	// scenario freezes.
	register(Scenario{Name: "transaction_deleted_account", Calls: func() []Call {
		const (
			opAccount  = "d0000000-0000-0000-0000-0000000000e1"
			opExpense  = "d0000000-0000-0000-0000-0000000000e2"
			opTransfer = "d0000000-0000-0000-0000-0000000000e3"
		)
		var acctID string
		return []Call{
			{Label: "create-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body:          map[string]any{"id": opAccount, "name": "Closed", "icon": "wallet", "currencyId": USD, "folderId": OwnerFolder},
				CaptureIDInto: &acctID},
			{Label: "delete-account", Method: "POST", Path: "/api/v1/account/delete-account", Auth: "owner",
				Body: map[string]any{"id": &acctID}},
			{Label: "err:create-expense-on-deleted", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": opExpense, "accountId": &acctID, "type": "expense",
					"amount": "5.00", "categoryId": CatFood, "date": "2024-04-04 10:00:00"}},
			{Label: "err:create-transfer-into-deleted", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": opTransfer, "accountId": OwnerAccount, "accountRecipientId": &acctID,
					"type": "transfer", "amount": "5.00", "date": "2024-04-04 10:00:00"}},
			{Label: "get-transaction-list-after", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})

	register(Scenario{Name: "transaction_import", Calls: func() []Call {
		body, ctype := buildImportBody()
		return []Call{
			{Label: "import-transaction-list", Method: "POST", Path: "/api/v1/transaction/import-transaction-list",
				Auth: "owner", RawBody: body, ContentType: ctype},
			{Label: "get-transaction-list-after", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})
}

// buildImportBody assembles the multipart form the import endpoint expects: a
// CSV file, a column-mapping JSON, and a fixed accountId override (so the
// mapping does not need an "account" column). The CSV amount is positive
// (income) and the category name matches the seeded CatFood so the row
// resolves to an existing category rather than creating a new one.
func buildImportBody() ([]byte, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "import.csv")
	if err != nil {
		panic(err)
	}
	if _, err := fw.Write([]byte("Date,Amount,Description,Category\n2026-01-15,12.34,coffee,Food\n")); err != nil {
		panic(err)
	}
	if err := w.WriteField("mapping", `{"date":"Date","amount":"Amount","description":"Description","category":"Category"}`); err != nil {
		panic(err)
	}
	if err := w.WriteField("accountId", OwnerAccount); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes(), w.FormDataContentType()
}
