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
				Body: map[string]any{"provider": "csv", "name": "Bank"}},
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
			// Txn1 itself was never touched by THIS conversion (it created a new
			// row instead); it shows the unrelated provenance seeded on it by the
			// SimpleFIN fixture (fixture.go's ImportRunSeeded) instead of an
			// empty list.
			{Label: "get-transaction-import-list-other-source", Method: "GET", Path: "/api/v1/import/get-transaction-import-list?transactionId=" + Txn1, Auth: "owner"},
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

	register(Scenario{Name: "import_simplefin_credentials", Calls: func() []Call {
		return []Call{
			{Label: "err:get-credential-key-none", Method: "GET", Path: "/api/v1/import/get-credential-key", Auth: "owner"},
			{Label: "set-credential-key", Method: "POST", Path: "/api/v1/import/set-credential-key", Auth: "owner",
				Body: map[string]any{"wrappedDataKey": "v1:aXY=:Y3Q=", "kdf": `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`}},
			{Label: "get-credential-key", Method: "GET", Path: "/api/v1/import/get-credential-key", Auth: "owner"},
			{Label: "err:set-credential-key-blank", Method: "POST", Path: "/api/v1/import/set-credential-key", Auth: "owner",
				Body: map[string]any{"wrappedDataKey": "", "kdf": ""}},
			{Label: "claim-setup-token", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": "aHR0cHM6Ly9icmlkZ2UuZXhhbXBsZS9zaW1wbGVmaW4vY2xhaW0vYWJj"}},
			{Label: "err:claim-setup-token-used", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": "used"}},
			{Label: "err:claim-setup-token-blank", Method: "POST", Path: "/api/v1/import/claim-setup-token", Auth: "guest",
				Body: map[string]any{"setupToken": ""}},
			// Guest connects a bank: the ciphertext is opaque to the server.
			{Label: "create-source-simplefin", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "My Bank", "credentialCiphertext": "v1:aXY=:Y3Q="}},
			{Label: "err:create-source-simplefin-no-ciphertext", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "My Bank"}},
			// Reconnect: same (user, provider) -> same source, new ciphertext + name.
			{Label: "create-source-simplefin-reconnect", Method: "POST", Path: "/api/v1/import/create-source", Auth: "guest",
				Body: map[string]any{"provider": "simplefin", "name": "My Bank (new)", "credentialCiphertext": "v1:aXYy:Y3Qy"}},
			{Label: "get-source-list-guest-bank", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "guest"},
		}
	}})

	register(Scenario{Name: "import_simplefin_sync", Calls: func() []Call {
		// ClockTime is "now"; the window is the last 30 days, which is where the
		// stub provider dates its rows. Request bodies are not part of the golden.
		day := func(offset int) string { return ClockTime.AddDate(0, 0, offset).Format("2006-01-02") }
		start, end := day(-30), day(0)
		return []Call{
			{Label: "list-external-accounts", Method: "POST", Path: "/api/v1/import/list-external-accounts", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL}},
			{Label: "err:list-external-accounts-bad-url", Method: "POST", Path: "/api/v1/import/list-external-accounts", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": "https://nope.example/"}},
			{Label: "err:list-external-accounts-push-source", Method: "POST", Path: "/api/v1/import/list-external-accounts", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourcePhone, "accessUrl": stubAccessURL}},
			// ACT-CHK is mapped -> 2 created; ACT-SAV unmapped -> 1 queued; one bridge warning -> partial.
			{Label: "sync-source", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start, "endDate": end}},
			// Same range again: every row is a duplicate, nothing is counted.
			{Label: "sync-source-again", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start, "endDate": end}},
			{Label: "err:sync-source-range", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": day(1), "endDate": start}},
			{Label: "err:sync-source-down", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "owner",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubDownURL, "startDate": start, "endDate": end}},
			{Label: "err:sync-source-foreign", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "guest",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start, "endDate": end}},
			{Label: "err:readonly-sync-source", Method: "POST", Path: "/api/v1/import/sync-source", Auth: "readonly",
				Body: map[string]any{"sourceId": ImportSourceBank, "accessUrl": stubAccessURL, "startDate": start}},
			{Label: "get-run-list", Method: "GET", Path: "/api/v1/import/get-run-list", Auth: "owner"},
			{Label: "get-run-list-by-source", Method: "GET", Path: "/api/v1/import/get-run-list?sourceId=" + ImportSourceBank, Auth: "owner"},
			{Label: "get-run-list-guest-empty", Method: "GET", Path: "/api/v1/import/get-run-list", Auth: "guest"},
			// The sync's run id is server-minted and Call.CaptureIDInto only reads
			// data.item.id, so get-run reads the SEEDED run (fixture.go) instead.
			{Label: "get-run", Method: "GET", Path: "/api/v1/import/get-run?id=" + ImportRunSeeded, Auth: "owner"},
			{Label: "err:get-run-foreign", Method: "GET", Path: "/api/v1/import/get-run?id=" + ImportRunSeeded, Auth: "guest"},
			{Label: "err:get-run-unknown", Method: "GET", Path: "/api/v1/import/get-run?id=00000000-0000-0000-0000-000000000000", Auth: "owner"},
			{Label: "get-source-list-after-sync", Method: "GET", Path: "/api/v1/import/get-source-list", Auth: "owner"},
			{Label: "get-queued-event-list-after-sync", Method: "GET", Path: "/api/v1/import/get-queued-event-list", Auth: "owner"},
		}
	}})
}
