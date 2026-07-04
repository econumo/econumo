package apiparity

// negative_paths pins validation-message and authorization-denial contracts not
// already covered by error_paths (unauthenticated 401 + blank category name are
// pinned there; this scenario deliberately does not repeat them). Every call
// carries the "err:" label prefix and every response must be a 4xx error
// envelope.
func init() {
	register(Scenario{Name: "negative_paths", Calls: func() []Call {
		return []Call{
			// Tier-2 length validation (frozen message), distinct from error_paths'
			// blank-name tier-1 case.
			{Label: "err:category-name-too-short", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000ef", "name": "ab", "type": "expense", "icon": "i"}},
			{Label: "err:order-empty-changes", Method: "POST", Path: "/api/v1/category/order-category-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{}}},
			{Label: "err:account-order-empty", Method: "POST", Path: "/api/v1/account/order-account-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{}}},
			{Label: "err:replace-folder-blank", Method: "POST", Path: "/api/v1/account/replace-folder", Auth: "owner",
				Body: map[string]any{"id": "", "replaceId": ""}},
			// Authorization: guest may not mutate owner's resources. icon/updatedAt
			// are required tier-1 fields (UpdateAccountRequest.Validate) — they must
			// be present so this fails in the service's ownership check
			// (errs.NewAccessDenied -> 403), not tier-1 validation.
			{Label: "err:guest-updates-owner-account", Method: "POST", Path: "/api/v1/account/update-account", Auth: "guest",
				Body: map[string]any{"id": OwnerAccount, "name": "Hacked", "icon": "hack", "updatedAt": "2024-01-01 00:00:00", "currencyId": USD}},
			{Label: "err:update-budget-bad-uuid", Method: "POST", Path: "/api/v1/user/update-budget", Auth: "owner",
				Body: map[string]any{"value": "not-a-uuid"}},
			// type is parsed before the write-access check, so an invalid type pins
			// the tier-2 "invalid choice" validation message, not an access denial.
			// Pins the frozen budget-name validation label at its CALL SITE — a
			// Phase 7 sed once corrupted the "Budget" label literal and no gate
			// caught it because this exact path was unpinned.
			{Label: "err:budget-name-too-short", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": "b0000000-0000-0000-0000-0000000000ee", "name": "ab", "currencyId": USD, "startDate": "2024-04-01"}},
			{Label: "err:update-tx-bad-type", Method: "POST", Path: "/api/v1/transaction/update-transaction", Auth: "owner",
				Body: map[string]any{"id": Txn1, "type": "bogus", "amount": "1.00", "accountId": OwnerAccount,
					"date": ClockTime.Format("2006-01-02 15:04:05")}},

			// ---- tier-1 (DecodeValidate) blank-field gaps: these endpoints' happy
			// paths are exercised elsewhere, but never their own validation branch. ----
			{Label: "err:create-account-blank", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{}},
			{Label: "err:update-account-blank", Method: "POST", Path: "/api/v1/account/update-account", Auth: "owner",
				Body: map[string]any{}},
			{Label: "err:delete-account-blank-id", Method: "POST", Path: "/api/v1/account/delete-account", Auth: "owner",
				Body: map[string]any{"id": ""}},
			{Label: "err:create-folder-blank-name", Method: "POST", Path: "/api/v1/account/create-folder", Auth: "owner",
				Body: map[string]any{"name": ""}},
			{Label: "err:update-folder-blank", Method: "POST", Path: "/api/v1/account/update-folder", Auth: "owner",
				Body: map[string]any{}},
			{Label: "err:hide-folder-blank-id", Method: "POST", Path: "/api/v1/account/hide-folder", Auth: "owner",
				Body: map[string]any{"id": ""}},
			{Label: "err:show-folder-blank-id", Method: "POST", Path: "/api/v1/account/show-folder", Auth: "owner",
				Body: map[string]any{"id": ""}},
			{Label: "err:update-category-blank", Method: "POST", Path: "/api/v1/category/update-category", Auth: "owner",
				Body: map[string]any{}},
			{Label: "err:archive-category-blank-id", Method: "POST", Path: "/api/v1/category/archive-category", Auth: "owner",
				Body: map[string]any{"id": ""}},
			{Label: "err:unarchive-category-blank-id", Method: "POST", Path: "/api/v1/category/unarchive-category", Auth: "owner",
				Body: map[string]any{"id": ""}},
			{Label: "err:delete-category-blank-id", Method: "POST", Path: "/api/v1/category/delete-category", Auth: "owner",
				Body: map[string]any{"id": "", "mode": "delete"}},

			// ---- tier-2 (service-level vo.ParseId) gaps: id/changes[].id passes
			// tier-1 NotBlank but is not a well-formed UUID, so the value-object
			// constructor inside the service rejects it ("invalid id") — a distinct
			// branch from both tier-1 validation and not-found/access-denied. ----
			{Label: "err:show-folder-bad-uuid", Method: "POST", Path: "/api/v1/account/show-folder", Auth: "owner",
				Body: map[string]any{"id": "not-a-uuid"}},
			{Label: "err:order-account-bad-uuid", Method: "POST", Path: "/api/v1/account/order-account-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": "not-a-uuid", "folderId": OwnerFolder, "position": 0}}}},
			{Label: "err:order-folder-bad-uuid", Method: "POST", Path: "/api/v1/account/order-folder-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": "not-a-uuid", "position": 0}}}},
			{Label: "err:archive-category-bad-uuid", Method: "POST", Path: "/api/v1/category/archive-category", Auth: "owner",
				Body: map[string]any{"id": "not-a-uuid"}},
			{Label: "err:unarchive-category-bad-uuid", Method: "POST", Path: "/api/v1/category/unarchive-category", Auth: "owner",
				Body: map[string]any{"id": "not-a-uuid"}},
			{Label: "err:delete-category-bad-uuid", Method: "POST", Path: "/api/v1/category/delete-category", Auth: "owner",
				Body: map[string]any{"id": "not-a-uuid", "mode": "delete"}},
			{Label: "err:order-category-bad-uuid", Method: "POST", Path: "/api/v1/category/order-category-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": "not-a-uuid", "position": 0}}}},
		}
	}})
}
