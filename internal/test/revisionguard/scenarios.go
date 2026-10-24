package revisionguard

import (
	ap "github.com/econumo/econumo/internal/test/apiparity"
)

// scenario is a guard-only request sequence. The parity catalogues pin wire
// shapes, so their classification merges and deletes run on fresh rows no
// transaction uses; these replay the same writes against rows that do carry
// transactions. Want lists the requests that must change some account's
// transaction data, so a scenario that stops reaching its write path fails
// instead of passing vacuously.
type scenario struct {
	Name  string
	Calls func() []ap.Call
	Want  []string
}

// Create endpoints take these as operation ids and mint the entity id, which
// the calls capture for later requests.
const (
	opCategory  = "c0000000-0000-0000-0000-0000000000f1"
	opPayee     = "20000000-0000-0000-0000-0000000000f1"
	opTag       = "10000000-0000-0000-0000-0000000000f1"
	opLabel     = "30000000-0000-0000-0000-0000000000f1"
	opTxn       = "d0000000-0000-0000-0000-0000000000f1"
	opRecurring = "e0000000-0000-0000-0000-0000000000f1"
	opAccount   = "a0000000-0000-0000-0000-0000000000f1"
	opAdoptee   = "d0000000-0000-0000-0000-0000000000f2"
	ruleID      = "0192b1e4-0000-7000-8000-00000000f001"
	txnDate     = "2024-04-04 10:00:00"
)

func post(label, path string, body map[string]any) ap.Call {
	return ap.Call{Label: label, Method: "POST", Path: path, Auth: "owner", Body: body}
}

func capture(c ap.Call, into *string) ap.Call {
	c.CaptureIDInto = into
	return c
}

func createExpense(extra map[string]any, into *string) ap.Call {
	body := map[string]any{"id": opTxn, "accountId": ap.OwnerAccount, "type": "expense", "amount": "5.00", "date": txnDate}
	for k, v := range extra {
		body[k] = v
	}
	return capture(post("create-transaction", "/api/v1/transaction/create-transaction", body), into)
}

// adoptee is a hand-entered expense the matcher will adopt for an Apple
// Wallet tap of the same amount: it falls on the tap's day (ClockTime),
// unlike Txn1, whose seed date is outside the match window.
func adoptee(amount string, into *string) ap.Call {
	return capture(post("create-adoptee", "/api/v1/transaction/create-transaction", map[string]any{
		"id": opAdoptee, "accountId": ap.OwnerAccount, "type": "expense", "amount": amount,
		"date": ap.ClockTime.Format("2006-01-02 15:04:05"), "description": "Shop",
	}), into)
}

func scenarios() []scenario {
	return []scenario{
		{
			Name: "move_transaction",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("move-within-day", "/api/v1/transaction/move-transaction", map[string]any{
						"id": ap.Txn1, "accountId": ap.OwnerAccount, "date": "2024-04-01", "aboveId": nil,
					}),
					post("move-to-another-day", "/api/v1/transaction/move-transaction", map[string]any{
						"id": ap.Txn1, "accountId": ap.OwnerAccount, "date": "2024-04-03", "aboveId": nil,
					}),
				}
			},
			Want: []string{"POST /api/v1/transaction/move-transaction"},
		},
		{
			Name: "category_merge_with_transactions",
			Calls: func() []ap.Call {
				var target string
				return []ap.Call{
					capture(post("create-target", "/api/v1/category/create-category", map[string]any{"id": opCategory, "name": "Groceries", "type": "expense", "icon": "cart"}), &target),
					post("merge-category", "/api/v1/category/merge-category", map[string]any{"sourceId": ap.CatFood, "targetId": &target}),
				}
			},
			Want: []string{"POST /api/v1/category/merge-category"},
		},
		{
			Name: "category_delete_with_transactions",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("delete-category", "/api/v1/category/delete-category", map[string]any{"id": ap.CatFood, "mode": "delete"}),
				}
			},
			Want: []string{"POST /api/v1/category/delete-category"},
		},
		{
			Name: "payee_merge_with_transactions",
			Calls: func() []ap.Call {
				var target string
				return []ap.Call{
					capture(post("create-target", "/api/v1/payee/create-payee", map[string]any{"id": opPayee, "name": "Grocery Store"}), &target),
					post("merge-payee", "/api/v1/payee/merge-payee", map[string]any{"sourceId": ap.PayeeShop, "targetId": &target}),
				}
			},
			Want: []string{"POST /api/v1/payee/merge-payee"},
		},
		{
			Name: "payee_delete_with_transactions",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("delete-payee", "/api/v1/payee/delete-payee", map[string]any{"id": ap.PayeeShop}),
				}
			},
			Want: []string{"POST /api/v1/payee/delete-payee"},
		},
		{
			Name: "tag_merge_with_transactions",
			Calls: func() []ap.Call {
				var txn, target string
				return []ap.Call{
					createExpense(map[string]any{"tagId": ap.TagWork}, &txn),
					capture(post("create-target", "/api/v1/tag/create-tag", map[string]any{"id": opTag, "name": "Office"}), &target),
					post("merge-tag", "/api/v1/tag/merge-tag", map[string]any{"sourceId": ap.TagWork, "targetId": &target}),
				}
			},
			Want: []string{"POST /api/v1/tag/merge-tag"},
		},
		{
			Name: "tag_delete_with_transactions",
			Calls: func() []ap.Call {
				var txn string
				return []ap.Call{
					createExpense(map[string]any{"tagId": ap.TagWork}, &txn),
					post("delete-tag", "/api/v1/tag/delete-tag", map[string]any{"id": ap.TagWork}),
				}
			},
			Want: []string{"POST /api/v1/tag/delete-tag"},
		},
		{
			Name: "label_merge_with_transactions",
			Calls: func() []ap.Call {
				var txn, target string
				return []ap.Call{
					createExpense(map[string]any{"labelIds": []string{ap.LabelWork}}, &txn),
					capture(post("create-target", "/api/v1/label/create-label", map[string]any{"id": opLabel, "name": "Office"}), &target),
					post("merge-label", "/api/v1/label/merge-label", map[string]any{"sourceId": ap.LabelWork, "targetId": &target}),
				}
			},
			Want: []string{"POST /api/v1/label/merge-label"},
		},
		{
			Name: "label_delete_with_transactions",
			Calls: func() []ap.Call {
				var txn string
				return []ap.Call{
					createExpense(map[string]any{"labelIds": []string{ap.LabelWork}}, &txn),
					post("delete-label", "/api/v1/label/delete-label", map[string]any{"id": ap.LabelWork}),
				}
			},
			Want: []string{"POST /api/v1/label/delete-label"},
		},
		{
			Name: "recurring_delete_with_linked_transaction",
			Calls: func() []ap.Call {
				var txn, recurring string
				return []ap.Call{
					createExpense(nil, &txn),
					capture(post("create-recurring-from-transaction", "/api/v1/recurring/create-recurring-transaction", map[string]any{
						"id": opRecurring, "type": "expense", "amount": "5.00", "accountId": ap.OwnerAccount,
						"schedule": "monthly", "nextPaymentAt": "2099-01-01 00:00:00", "description": "",
						"sourceTransactionId": &txn,
					}), &recurring),
					post("delete-recurring", "/api/v1/recurring/delete-recurring-transaction", map[string]any{"id": &recurring}),
				}
			},
			Want: []string{"POST /api/v1/recurring/delete-recurring-transaction"},
		},
		{
			Name: "account_balance_correction",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("update-account-balance", "/api/v1/account/update-account", map[string]any{
						"id": ap.OwnerAccount, "name": "Main", "icon": "wallet", "balance": "500", "updatedAt": "2024-01-01 00:00:00",
					}),
				}
			},
			Want: []string{"POST /api/v1/account/update-account"},
		},
		{
			Name: "account_delete_zeroes_balance",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("delete-account", "/api/v1/account/delete-account", map[string]any{"id": ap.OwnerAccount}),
				}
			},
			Want: []string{"POST /api/v1/account/delete-account"},
		},
		{
			Name: "transaction_moves_between_accounts",
			Calls: func() []ap.Call {
				var account string
				return []ap.Call{
					capture(post("create-account", "/api/v1/account/create-account", map[string]any{"id": opAccount, "name": "Second", "icon": "bank", "currencyId": ap.USD, "folderId": ap.OwnerFolder}), &account),
					post("move-transaction", "/api/v1/transaction/update-transaction", map[string]any{
						"id": ap.Txn1, "type": "expense", "amount": "12.50", "accountId": &account,
						"categoryId": ap.CatFood, "payeeId": ap.PayeeShop, "date": txnDate, "description": "lunch",
					}),
				}
			},
			Want: []string{"POST /api/v1/transaction/update-transaction"},
		},
		{
			Name: "import_apply_rule_classifies",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("create-rule", "/api/v1/import/create-rule", map[string]any{
						"id": ruleID, "action": "classify", "matchField": "external_payee", "matchType": "contains",
						"matchValue": "Employer", "payeeId": ap.PayeeShop, "priority": 1,
					}),
					// The seeded import link carries no applied snapshot, so the
					// row counts as edited and only includeEdited reaches it.
					post("apply-rule", "/api/v1/import/apply-rule", map[string]any{"ruleId": ruleID, "scope": "all", "includeEdited": true}),
				}
			},
			Want: []string{"POST /api/v1/import/apply-rule"},
		},
		{
			// Txn2 carries a link from each seeded source, so only deleting the
			// second one flips it back to hand-entered.
			Name: "import_delete_sources",
			Calls: func() []ap.Call {
				return []ap.Call{
					post("delete-phone-source", "/api/v1/import/delete-source", map[string]any{"id": ap.ImportSourcePhone}),
					post("delete-bank-source", "/api/v1/import/delete-source", map[string]any{"id": ap.ImportSourceBank}),
				}
			},
			Want: []string{"POST /api/v1/import/delete-source"},
		},
		{
			// Mapping the card converts its queued 12.50 tap by adopting the
			// existing transaction: the link gains a transaction id and nothing
			// else in the request touches that transaction.
			Name: "import_link_account_adopts",
			Calls: func() []ap.Call {
				var txn string
				return []ap.Call{
					adoptee("12.50", &txn),
					post("link-account", "/api/v1/import/link-account", map[string]any{"sourceId": ap.ImportSourcePhone, "externalAccountId": "wallet", "accountId": ap.OwnerAccount}),
				}
			},
			Want: []string{"POST /api/v1/import/link-account"},
		},
		{
			// A push for a mapped card adopts the matching transaction, so the
			// only write that touches it is the new link row.
			Name: "import_ingest_adopts",
			Calls: func() []ap.Call {
				var txn string
				return []ap.Call{
					post("link-account", "/api/v1/import/link-account", map[string]any{"sourceId": ap.ImportSourcePhone, "externalAccountId": "wallet", "accountId": ap.OwnerAccount}),
					adoptee("7.77", &txn),
					{Label: "ingest-adopting", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
						RawBody: []byte(`{"account":"wallet","payee":"Shop","amount":"7.77","currency":"USD","eventId":"tap-guard"}`), ContentType: "application/json"},
				}
			},
			Want: []string{"POST /api/v1/import/ingest-apple-wallet-event"},
		},
	}
}
