package mcpparity

// The MCP scenario catalogue: JSON-RPC call sequences replayed against the
// live /mcp endpoint (the production handler, server.BuildAPI), on top of the
// apiparity.Seed fixture. Body shapes for the REST seeding steps are copied
// from the existing apiparity catalogue files (internal/test/apiparity/*.go)
// so the two suites stay in lockstep with the real endpoints' validation.

import "github.com/econumo/econumo/internal/test/apiparity"

func init() {
	register(Scenario{Name: "lifecycle", Steps: []Step{
		{Label: "initialize", RPC: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"mcpparity","version":"1"}}}`},
		{Label: "tools-list", RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`},
		{Label: "prompts-list", RPC: `{"jsonrpc":"2.0","id":3,"method":"prompts/list","params":{}}`},
	}})

	// unauthorized exercises the auth gate: /mcp sits behind the same bearer
	// middleware as the REST API, so a missing token never reaches the MCP
	// server — it gets the standard 401 envelope, not a JSON-RPC error.
	register(Scenario{Name: "unauthorized", Steps: []Step{
		{Label: "initialize-no-auth", NoAuth: true,
			RPC: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"mcpparity","version":"1"}}}`},
	}})

	// reference_tools REST-seeds one fresh category/tag/payee/account (on top
	// of the apiparity fixture's own category/tag/payee/account) so every
	// list_*/get_user tool reflects both the seeded fixture AND a
	// just-written row, then calls all eight reference-data tools.
	register(Scenario{Name: "reference_tools", Steps: []Step{
		{Label: "seed-category", Method: "POST", Path: "/api/v1/category/create-category",
			Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000c1", "name": "MCP Category", "type": "expense", "icon": "tag"}},
		{Label: "seed-tag", Method: "POST", Path: "/api/v1/tag/create-tag",
			Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000c1", "name": "MCP Tag"}},
		{Label: "seed-payee", Method: "POST", Path: "/api/v1/payee/create-payee",
			Body: map[string]any{"id": "20000000-0000-0000-0000-0000000000c1", "name": "MCP Payee"}},
		{Label: "seed-account", Method: "POST", Path: "/api/v1/account/create-account",
			Body: map[string]any{"id": "a0000000-0000-0000-0000-0000000000c1", "name": "MCP Account", "icon": "bank", "currencyId": apiparity.USD, "folderId": apiparity.OwnerFolder}},
		{Label: "list-accounts", RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_accounts","arguments":{}}}`},
		{Label: "list-categories", RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_categories","arguments":{}}}`},
		{Label: "list-tags", RPC: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_tags","arguments":{}}}`},
		{Label: "list-payees", RPC: `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_payees","arguments":{}}}`},
		{Label: "list-currencies", RPC: `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_currencies","arguments":{}}}`},
		{Label: "list-budgets", RPC: `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_budgets","arguments":{}}}`},
		{Label: "get-user", RPC: `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_user","arguments":{}}}`},
		{Label: "list-connections", RPC: `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"list_connections","arguments":{}}}`},
	}})

	// reference_tools_write REST-seeds one category/tag/payee per domain,
	// CAPTURING each REST response's minted entity id (create-category/-tag/-payee
	// treat the body's "id" as an OPERATION id only, per
	// internal/{category,tag,payee}/create.go — the entity always gets a fresh
	// server-minted UUIDv7, unlike create-budget), then substitutes that id
	// (%s) into the update/set-archived RPC calls for the SAME domain before the
	// next seed step overwrites the single capture slot. Each domain's
	// create_* MCP call stands alone (its response only carries the new id
	// inside the normalized-away structuredContent, so there's no capture
	// mechanism for chaining further off it — same reasoning as the
	// "transactions" scenario above). Ends with one domain-error path
	// (create_category with a too-short name).
	register(Scenario{Name: "reference_tools_write", Steps: []Step{
		{Label: "seed-category", Method: "POST", Path: "/api/v1/category/create-category", CaptureID: true,
			Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000d1", "name": "MCP Write Category", "type": "expense", "icon": "tag"}},
		{Label: "create-category", RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_category","arguments":{"name":"MCP New Category","type":"expense","icon":"coffee"}}}`},
		{Label: "update-category", RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"update_category","arguments":{"id":"%s","name":"MCP Category Renamed","icon":"star"}}}`},
		{Label: "set-category-archived", RPC: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"set_category_archived","arguments":{"id":"%s","archived":true}}}`},

		{Label: "seed-tag", Method: "POST", Path: "/api/v1/tag/create-tag", CaptureID: true,
			Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000d1", "name": "MCP Write Tag"}},
		{Label: "create-tag", RPC: `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_tag","arguments":{"name":"MCP New Tag"}}}`},
		{Label: "update-tag", RPC: `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"update_tag","arguments":{"id":"%s","name":"MCP Tag Renamed"}}}`},
		{Label: "set-tag-archived", RPC: `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"set_tag_archived","arguments":{"id":"%s","archived":true}}}`},

		{Label: "seed-payee", Method: "POST", Path: "/api/v1/payee/create-payee", CaptureID: true,
			Body: map[string]any{"id": "20000000-0000-0000-0000-0000000000d1", "name": "MCP Write Payee"}},
		{Label: "create-payee", RPC: `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"create_payee","arguments":{"name":"MCP New Payee"}}}`},
		{Label: "update-payee", RPC: `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"update_payee","arguments":{"id":"%s","name":"MCP Payee Renamed"}}}`},
		{Label: "set-payee-archived", RPC: `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"set_payee_archived","arguments":{"id":"%s","archived":true}}}`},

		{Label: "list-categories", RPC: `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"list_categories","arguments":{}}}`},
		{Label: "list-tags", RPC: `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"list_tags","arguments":{}}}`},
		{Label: "list-payees", RPC: `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"list_payees","arguments":{}}}`},

		{Label: "create-category-short-name", RPC: `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"create_category","arguments":{"name":"ab","type":"expense"}}}`},
	}})

	// budget REST-creates a budget then drives get_budget for a valid and an
	// invalid month. Unlike category/tag/payee/account/transaction,
	// create-budget honors the client-supplied id verbatim (it isn't an
	// operation/idempotency id, see internal/budget/create.go) and its
	// response is {item: {meta, filters, balances, ...}} rather than the
	// {item: {id, ...}} shape extractItemID expects — so the seeded id is
	// referenced directly (mcpBudgetID) instead of via CaptureID.
	const mcpBudgetID = "b0000000-0000-0000-0000-0000000000c1"
	register(Scenario{Name: "budget", Steps: []Step{
		{Label: "seed-budget", Method: "POST", Path: "/api/v1/budget/create-budget",
			Body: map[string]any{"id": mcpBudgetID, "name": "MCP Budget", "currencyId": apiparity.USD, "startDate": "2024-04-01"}},
		{Label: "get-budget",
			RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_budget","arguments":{"budget_id":"` + mcpBudgetID + `","month":"2024-04"}}}`},
		{Label: "get-budget-bad-month",
			RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_budget","arguments":{"budget_id":"` + mcpBudgetID + `","month":"junk"}}}`},
	}})

	// budget_write drives all eight budget create/configure MCP tools end to
	// end, purely via MCP calls on top of the apiparity fixture (owner's seeded
	// account + categories). create_budget/create_folder/create_envelope mint
	// their own entity ids server-side (internal/budget/mcp/mcp.go), unlike the
	// REST create-* endpoints, so each id is captured out of the tool's
	// structuredContent (extractMCPID) and threaded into later steps via named
	// {{vars}} rather than the single-slot %s convention the other scenarios
	// use — budget_id alone is needed by six later steps, which a single slot
	// can't hold alongside folder_id/element_id. The update_budget step
	// exercises the ExcludedAccounts round-trip fix directly: it runs AFTER
	// set_budget_account_included excludes the owner's account, and the closing
	// get_budget asserts the exclusion survived the rename (had the tool passed
	// an empty ExcludedAccounts list, UpdateBudget's authoritative-replace
	// semantics — internal/budget/crud.go — would have silently re-included it).
	// Ends with one domain-error path: set_limit for a month before the
	// budget's start.
	register(Scenario{Name: "budget_write", Steps: []Step{
		{Label: "create-budget", CaptureAs: "budget_id", MCPCapturePath: []string{"item", "meta", "id"},
			RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_budget","arguments":{"name":"MCP New Budget","currency_id":"` + apiparity.USD + `","start_date":"2024-05-01"}}}`},
		{Label: "create-folder", CaptureAs: "folder_id", MCPCapturePath: []string{"item", "id"},
			RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_folder","arguments":{"budget_id":"{{budget_id}}","name":"Bills"}}}`},
		{Label: "update-folder",
			RPC: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"update_folder","arguments":{"budget_id":"{{budget_id}}","id":"{{folder_id}}","name":"Bills Renamed"}}}`},
		{Label: "create-envelope", CaptureAs: "element_id", MCPCapturePath: []string{"item", "id"},
			RPC: `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_envelope","arguments":{"budget_id":"{{budget_id}}","name":"Groceries","icon":"cart","currency_id":"` + apiparity.USD + `","folder_id":"{{folder_id}}","category_ids":["` + apiparity.CatFood + `"]}}}`},
		{Label: "update-envelope",
			RPC: `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"update_envelope","arguments":{"budget_id":"{{budget_id}}","id":"{{element_id}}","name":"Groceries Renamed","icon":"cart","currency_id":"` + apiparity.USD + `","category_ids":["` + apiparity.CatFood + `","` + apiparity.CatSalary + `"],"archived":false}}}`},
		{Label: "set-limit",
			RPC: `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"set_limit","arguments":{"budget_id":"{{budget_id}}","element_id":"{{element_id}}","month":"2024-05","amount":"150.00"}}}`},
		{Label: "set-budget-account-included",
			RPC: `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"set_budget_account_included","arguments":{"budget_id":"{{budget_id}}","account_id":"` + apiparity.OwnerAccount + `","included":false}}}`},
		{Label: "update-budget",
			RPC: `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"update_budget","arguments":{"budget_id":"{{budget_id}}","name":"MCP New Budget Renamed","currency_id":"` + apiparity.USD + `"}}}`},
		{Label: "get-budget-after",
			RPC: `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"get_budget","arguments":{"budget_id":"{{budget_id}}","month":"2024-05"}}}`},
		{Label: "set-limit-before-start",
			RPC: `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"set_limit","arguments":{"budget_id":"{{budget_id}}","element_id":"{{element_id}}","month":"2024-01","amount":"10.00"}}}`},
	}})

	// transactions REST-seeds a FRESH account (so list_transactions starts
	// empty rather than inheriting the fixture's two seeded transactions),
	// captures its minted id, then drives create_transaction / list_transactions
	// and an error-path create with a bogus category id. The scenario ends
	// there rather than adding a delete_transaction step: create_transaction's
	// MCP response only carries the new id inside the (normalized-away)
	// structuredContent, so capturing it back out for a delete step would need
	// a second extraction convention; update/delete are already covered by the
	// transaction feature's own MCP tests (internal/transaction/mcp/mcp_test.go).
	register(Scenario{Name: "transactions", Steps: []Step{
		{Label: "seed-account", Method: "POST", Path: "/api/v1/account/create-account", CaptureID: true,
			Body: map[string]any{"id": "a0000000-0000-0000-0000-0000000000c2", "name": "MCP Wallet", "icon": "wallet", "currencyId": apiparity.USD, "folderId": apiparity.OwnerFolder}},
		{Label: "create-transaction",
			RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_transaction","arguments":{"type":"expense","amount":"12.50","account_id":"%s","date":"2024-04-02","category_id":"` + apiparity.CatFood + `"}}}`},
		{Label: "list-transactions",
			RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"%s"}}}`},
		{Label: "create-transaction-bogus-category",
			RPC: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_transaction","arguments":{"type":"expense","amount":"9.00","account_id":"%s","date":"2024-04-02","category_id":"00000000-0000-0000-0000-000000000000"}}}`},
		{Label: "list-transactions-account-period",
			RPC: `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"%s","period_start":"2024-04-02","period_end":"2024-04-02"}}}`},
	}})

	// transaction_review drives the MCP-only richer list_transactions filters
	// (uncategorized/category/payee/tag) and bulk_update_transactions. It seeds
	// two fresh accounts (so results are isolated from the base fixture's own
	// Txn1/Txn2 on OwnerAccount, and a transfer needs a distinct recipient
	// account), reuses the fixture's already-owned CatFood/PayeeShop/TagWork
	// (no need to seed fresh reference data), then: creates a categorized
	// expense and an uncategorized transfer (transfers never carry a category,
	// per internal/transaction/usecase.go's buildState — the only way to
	// produce an uncategorized row via the tools), exercises each filter alone,
	// bulk-sets a tag on the expense, re-lists by tag to confirm, then ends with
	// bulk_update_transactions rejecting a category on the transfer (the
	// category-on-transfer invariant bulk enforces that a full update would
	// have silently dropped instead — see internal/transaction/bulk.go).
	register(Scenario{Name: "transaction_review", Steps: []Step{
		{Label: "seed-account-1", CaptureAs: "account1_id", CaptureID: true, Method: "POST", Path: "/api/v1/account/create-account",
			Body: map[string]any{"id": "a0000000-0000-0000-0000-0000000000c3", "name": "MCP Review Wallet 1", "icon": "wallet", "currencyId": apiparity.USD, "folderId": apiparity.OwnerFolder}},
		{Label: "seed-account-2", CaptureAs: "account2_id", CaptureID: true, Method: "POST", Path: "/api/v1/account/create-account",
			Body: map[string]any{"id": "a0000000-0000-0000-0000-0000000000c4", "name": "MCP Review Wallet 2", "icon": "wallet", "currencyId": apiparity.USD, "folderId": apiparity.OwnerFolder}},
		{Label: "create-expense", CaptureAs: "tx1_id", MCPCapturePath: []string{"item", "id"},
			RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_transaction","arguments":{"type":"expense","amount":"12.50","account_id":"{{account1_id}}","date":"2024-04-02","category_id":"` + apiparity.CatFood + `","payee_id":"` + apiparity.PayeeShop + `"}}}`},
		{Label: "create-transfer", CaptureAs: "tx2_id", MCPCapturePath: []string{"item", "id"},
			RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_transaction","arguments":{"type":"transfer","amount":"5.00","account_id":"{{account1_id}}","account_recipient_id":"{{account2_id}}","date":"2024-04-02"}}}`},
		{Label: "list-by-category",
			RPC: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"{{account1_id}}","category_id":"` + apiparity.CatFood + `"}}}`},
		{Label: "list-uncategorized",
			RPC: `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"{{account1_id}}","uncategorized":true}}}`},
		{Label: "list-by-payee",
			RPC: `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"{{account1_id}}","payee_id":"` + apiparity.PayeeShop + `"}}}`},
		{Label: "list-uncategorized-and-category-conflict",
			RPC: `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"{{account1_id}}","uncategorized":true,"category_id":"` + apiparity.CatFood + `"}}}`},
		{Label: "bulk-set-tag",
			RPC: `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"bulk_update_transactions","arguments":{"ids":["{{tx1_id}}"],"tag_id":"` + apiparity.TagWork + `"}}}`},
		{Label: "list-by-tag",
			RPC: `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"list_transactions","arguments":{"account_id":"{{account1_id}}","tag_id":"` + apiparity.TagWork + `"}}}`},
		{Label: "bulk-category-on-transfer-error",
			RPC: `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"bulk_update_transactions","arguments":{"ids":["{{tx2_id}}"],"category_id":"` + apiparity.CatFood + `"}}}`},
	}})

	register(Scenario{Name: "prompts", Steps: []Step{
		{Label: "get-log-expense", RPC: `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"log-expense","arguments":{"description":"27.50 groceries at Lidl yesterday"}}}`},
		{Label: "get-budget-review", RPC: `{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"budget-review","arguments":{}}}`},
		{Label: "get-budget-setup", RPC: `{"jsonrpc":"2.0","id":3,"method":"prompts/get","params":{"name":"budget-setup","arguments":{"name":"Household"}}}`},
		{Label: "get-budget-update", RPC: `{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"budget-update","arguments":{}}}`},
	}})
}
