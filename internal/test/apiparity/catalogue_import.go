package apiparity

// Apple Wallet import: source lifecycle, card mapping with its conversion
// run, the ingest route under both token scopes, queue triage, and the
// provenance read. Every scenario starts from the fresh seed (ImportSourcePhone
// owned by Owner with one linked tap on Txn2, one queued tap on the "wallet"
// card, one queued EUR tap on "eurocard", one failed event).
func init() {
	register(Scenario{Name: "import_sources", Calls: func() []Call {
		return []Call{
			{Label: "get-source-list", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "owner"},
			// Owner already has an apple-wallet source: create is idempotent per (user, provider) and returns it.
			{Label: "create-source-idempotent", Method: "POST", Path: "/api/v1/import/create-source", Auth: "owner",
				Body: map[string]any{"provider": "apple-wallet", "name": "Second phone"}},
			{Label: "err:create-source-unknown-provider", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "Bank"}},
			{Label: "create-source", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "apple-wallet", "name": "Guest phone"}},
			{Label: "get-source-list-guest", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "guest"},
			// Guest may not delete the owner's source (looked up by owner -> coded 400, no existence leak).
			{Label: "err:delete-source-foreign", Method: "POST", Path: "/api/v1/import/delete-source", Auth: "guest",
				Body: map[string]any{"id": ImportSourcePhone}},
			{Label: "delete-source", Method: "POST", Path: "/api/v1/import/delete-source", Auth: "owner",
				Body: map[string]any{"id": ImportSourcePhone}},
			// The cascade dropped the ledger row behind Txn2, so it reads as hand-entered now.
			{Label: "get-transaction-list-after-delete", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})

	register(Scenario{Name: "import_account_links", Calls: func() []Call {
		return []Call{
			{Label: "err:link-account-currency-mismatch", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "eurocard", "accountId": OwnerAccount}},
			// Guest-owned SharedAccount is shared with Owner but not OWNED by Owner: not mappable.
			{Label: "err:link-account-not-owned", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": SharedAccount}},
			// Mapping "wallet" converts its queued tap: Txn1's fixed 2024-04-01
			// seed date is outside the matcher's window around ClockTime, so
			// the run CREATES a transaction (run.importedCount 1) rather than
			// adopting Txn1.
			{Label: "link-account", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": OwnerAccount}},
			{Label: "err:link-account-again", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": OwnerAccount}},
			{Label: "get-queued-event-list-after-link", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
			// Txn1 itself was never touched by the conversion (it created a new
			// row instead), so its provenance list is empty.
			{Label: "get-transaction-import-list-no-provenance", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=" + Txn1, Auth: "owner"},
			{Label: "ignore-account", Method: "POST", Path: "/api/v1/import/ignore-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "eurocard"}},
			// "map instead" over an ignored card is allowed; currency still has to agree, so it stays refused for eurocard.
			{Label: "err:link-ignored-card-currency-mismatch", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "eurocard", "accountId": OwnerAccount}},
			{Label: "unlink-account", Method: "POST", Path: "/api/v1/import/unlink-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet"}},
			{Label: "unlink-account-unknown-card-noop", Method: "POST", Path: "/api/v1/import/unlink-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "nope"}},
			{Label: "get-source-list-after-unlink", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "owner"},
		}
	}})

	register(Scenario{Name: "import_ingest", Calls: func() []Call {
		return []Call{
			// Unmapped card -> queued, under the ingest-scoped PAT.
			{Label: "ingest-queued", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`{"account":"wallet","payee":"Coffee Corner","amount":"3.25","currency":"USD","eventId":"tap-3"}`), ContentType: "application/json"},
			{Label: "ingest-duplicate", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`{"account":"wallet","payee":"Coffee Corner","amount":"3.25","currency":"USD","eventId":"tap-3"}`), ContentType: "application/json"},
			{Label: "link-account", Method: "POST", Path: "/api/v1/import/link-account", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "externalAccountId": "wallet", "accountId": OwnerAccount}},
			// Mapped card, no candidate -> created; a full-scope token may push too.
			{Label: "ingest-created", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "owner",
				RawBody: []byte(`{"account":"Wallet","payee":"Grocer","amount":"$41.10","currency":"usd","occurredAt":"2026-08-20T10:42:03-07:00","eventId":"tap-4"}`), ContentType: "application/json"},
			{Label: "ingest-failed-payload", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`{"account":"wallet","amount":"free","currency":"USD"}`), ContentType: "application/json"},
			{Label: "ingest-not-json", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "ingest",
				RawBody: []byte(`not json at all`), ContentType: "text/plain"},
			{Label: "get-queued-event-list", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
			{Label: "get-transaction-list", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
			// Guest has no source: coded 400 (import.source_not_found).
			{Label: "err:ingest-no-source", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "guest",
				RawBody: []byte(`{"account":"x","amount":"1","currency":"USD"}`), ContentType: "application/json"},
			// Scope + access enforcement live in the middleware; both envelopes are frozen.
			{Label: "err:ingest-token-on-management-route", Method: "POST", Path: "/api/v1/import/create-source", Auth: "ingest",
				Body: map[string]any{"provider": "apple-wallet", "name": "x"}},
			{Label: "err:ingest-token-on-read-route", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "ingest"},
			{Label: "err:readonly-ingest", Method: "POST", Path: "/api/v1/import/ingest-apple-wallet-event", Auth: "readonly",
				RawBody: []byte(`{"account":"x","amount":"1","currency":"USD"}`), ContentType: "application/json"},
		}
	}})

	register(Scenario{Name: "import_queue", Calls: func() []Call {
		return []Call{
			{Label: "get-queued-event-list", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
			{Label: "skip-queued-event", Method: "POST", Path: "/api/v1/import/skip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "err:skip-queued-event-again", Method: "POST", Path: "/api/v1/import/skip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "unskip-queued-event", Method: "POST", Path: "/api/v1/import/unskip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "err:unskip-queued-event-again", Method: "POST", Path: "/api/v1/import/unskip-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			{Label: "err:skip-foreign-link", Method: "POST", Path: "/api/v1/import/skip-queued-event", Auth: "guest",
				Body: map[string]any{"linkId": ImportLinkQueued}},
			// Manual import of the queued tap: the SPA prefills create-transaction from the row.
			// "id" in the transaction body is the idempotency operation key only
			// (create-transaction always mints a fresh entity id), so the
			// resulting transaction's real id is not knowable here to reference
			// in a later GET's query string — the provenance read below uses
			// Txn2 (already linked by the base seed) instead.
			{Label: "import-queued-event", Method: "POST", Path: "/api/v1/import/import-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued, "transaction": map[string]any{
					"id": "b0000000-0000-0000-0000-0000000000f1", "type": "expense", "accountId": OwnerAccount,
					"amount": "12.50", "categoryId": CatFood, "payeeId": PayeeShop, "date": "2026-08-20 17:42:03", "description": "Shop (wallet)", "labelIds": []string{}}}},
			{Label: "err:import-queued-event-again", Method: "POST", Path: "/api/v1/import/import-queued-event", Auth: "owner",
				Body: map[string]any{"linkId": ImportLinkQueued, "transaction": map[string]any{
					"id": "b0000000-0000-0000-0000-0000000000f2", "type": "expense", "accountId": OwnerAccount,
					"amount": "12.50", "date": "2026-08-20 17:42:03", "labelIds": []string{}}}},
			{Label: "get-transaction-import-list", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=" + Txn2, Auth: "owner"},
			{Label: "err:get-transaction-import-list-blank", Method: "GET", Path: "/api/v1/import/get-transaction-import-list", Auth: "owner"},
			{Label: "get-transaction-import-list-foreign-empty", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=" + Txn2, Auth: "guest"},
			// Failed event: retry re-parses the stored payload (still broken -> failed again), then discard.
			{Label: "retry-event", Method: "POST", Path: "/api/v1/import/retry-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventFailed}},
			{Label: "err:retry-event-not-failed", Method: "POST", Path: "/api/v1/import/retry-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventQueued}},
			{Label: "discard-event", Method: "POST", Path: "/api/v1/import/discard-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventFailed}},
			{Label: "err:discard-event-gone", Method: "POST", Path: "/api/v1/import/discard-event", Auth: "owner",
				Body: map[string]any{"eventId": ImportEventFailed}},
			{Label: "get-queued-event-list-final", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
		}
	}})
}
