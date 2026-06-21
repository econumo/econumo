package main

// Connection mutation comparison cases.
//
// The connection invite/disconnect routes are now implemented in BOTH backends:
// the Go reimplementation ports the EconumoCloudBundle routes (which route to the
// open-source ConnectionInviteService / ConnectionService logic). So
// generate-invite + delete-invite ARE compared for real here; accept-invite and
// delete-connection cannot be fairly compared (see their per-case reasons).
//
// set-account-access and revoke-account-access have always been live in both;
// the seed holds a kuznetsov2d <-> Irina connection with shared accounts in both
// directions, so they are single-user deterministic mutations (the owner changes
// /revokes the other user's access to an owned account), with get-connection-list
// as the state read.
//
// NOTE: the get-connection-list sharedAccounts array has a documented PHP-vs-Go
// ordering difference; the harness canonical compare sorts arrays, so only
// genuine content differences register.

const connectionList = "/api/v1/connection/get-connection-list"

// connectedAccount picks a kuznetsov2d-OWNED account that the connected user
// (Irina) currently has access to (want=true) or does NOT have access to
// (want=false), reading it straight from get-connection-list so the case stays
// robust to seed changes. Returns the account id, the connected user id, and the
// current role (if any). It identifies "owned by me" via ownerUserId == my id.
func connectedAccount(php *client, want bool) (accountID, otherUserID, role, skip string, err error) {
	me, err := php.userID()
	if err != nil {
		return "", "", "", "", err
	}
	data, err := php.getData(connectionList, nil)
	if err != nil {
		return "", "", "", "", err
	}
	root, _ := data.(map[string]any)
	items, _ := root["items"].([]any)
	for _, raw := range items {
		conn, _ := raw.(map[string]any)
		user, _ := conn["user"].(map[string]any)
		uid, _ := user["id"].(string)
		if uid == "" {
			continue
		}
		shared, _ := conn["sharedAccounts"].([]any)
		// Collect the set of my-owned accounts this connected user already holds.
		held := map[string]string{} // accountId -> role
		for _, sraw := range shared {
			sa, _ := sraw.(map[string]any)
			owner, _ := sa["ownerUserId"].(string)
			aid, _ := sa["id"].(string)
			r, _ := sa["role"].(string)
			if owner == me && aid != "" {
				held[aid] = r
			}
		}
		if want {
			for aid, r := range held {
				return aid, uid, r, "", nil
			}
		} else {
			// Need a my-owned account NOT in held. Pull my owned accounts from the
			// account list and pick the first one this user does not hold.
			accts, aerr := php.items(acctList, nil)
			if aerr != nil {
				return "", "", "", "", aerr
			}
			for _, a := range accts {
				ow, _ := a["owner"].(map[string]any)
				if oid, _ := ow["id"].(string); oid != me {
					continue
				}
				aid, _ := a["id"].(string)
				if aid == "" {
					continue
				}
				if _, ok := held[aid]; !ok {
					return aid, uid, "", "", nil
				}
			}
		}
	}
	if want {
		return "", "", "", "no connected user with a shared (my-owned) account in " + connectionList, nil
	}
	return "", "", "", "no my-owned account ungranted to a connected user", nil
}

func connectionCases() []mutationCase {
	state := func(php *client) string { return connectionList }
	return []mutationCase{
		{
			// Update an EXISTING grant's role (the affected user already has access
			// to this owned account). Pure role flip -> no folder/options seeding.
			name: "connection/set-account-access-update",
			build: func(php *client) (string, map[string]any, string, error) {
				accountID, otherUserID, role, skip, err := connectedAccount(php, true)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				// Choose a DIFFERENT role from the current one so a real mutation
				// occurs and the state read reflects it.
				newRole := "admin"
				if role == "admin" {
					newRole = "user"
				}
				return "/api/v1/connection/set-account-access", map[string]any{
					"accountId": accountID,
					"userId":    otherUserID,
					"role":      newRole,
				}, "", nil
			},
			stateRead: state,
		},
		{
			// FRESH grant to the connected user on an owned account they don't yet
			// hold. Exercises the PHP side effects Go mirrors: seed the affected
			// user's accounts_options at max+1 and add the account to their last
			// folder. The state read (connection-list) shows the new sharedAccounts
			// entry; folder/options effects are not exposed there but are covered by
			// Go unit/integration tests.
			name: "connection/set-account-access-grant",
			build: func(php *client) (string, map[string]any, string, error) {
				accountID, otherUserID, _, skip, err := connectedAccount(php, false)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/connection/set-account-access", map[string]any{
					"accountId": accountID,
					"userId":    otherUserID,
					"role":      "admin",
				}, "", nil
			},
			stateRead: state,
		},
		{
			// Revoke an EXISTING grant. Mirrors PHP revokeAccountAccess: drops the
			// grant + the affected user's accounts_options row + folder membership.
			// The connection-list state read shows the account removed from that
			// user's sharedAccounts.
			name: "connection/revoke-account-access",
			build: func(php *client) (string, map[string]any, string, error) {
				accountID, otherUserID, _, skip, err := connectedAccount(php, true)
				if skip != "" || err != nil {
					return "", nil, skip, err
				}
				return "/api/v1/connection/revoke-account-access", map[string]any{
					"accountId": accountID,
					"userId":    otherUserID,
				}, "", nil
			},
			stateRead: state,
		},
		{
			// Generate the user's invite. Response is {item:{code,expiredAt}} where
			// code is random (5 chars) and expiredAt is now+5min — both differ per
			// backend, so they are masked; the comparison then verifies the
			// envelope + item SHAPE matches. No state read (the invite is not
			// exposed via any GET).
			name: "connection/generate-invite",
			build: func(php *client) (string, map[string]any, string, error) {
				return "/api/v1/connection/generate-invite", map[string]any{}, "", nil
			},
			volatile: []string{"code", "expiredAt"},
		},
		{
			// Clear the user's invite. Response is the empty {} on both backends;
			// idempotent (no-op if none). No state read.
			name: "connection/delete-invite",
			build: func(php *client) (string, map[string]any, string, error) {
				return "/api/v1/connection/delete-invite", map[string]any{}, "", nil
			},
		},
		{
			// accept-invite redeems a CODE. Each backend mints its own random code,
			// so a single request body (the harness sends the same body to both)
			// cannot be valid on both at once — the one-body-for-both model can't
			// express this two-step, per-backend-code exchange. The full happy path
			// is covered by the Go end-to-end test (TestAcceptInvite_ConnectsUsers).
			name: "connection/accept-invite",
			build: func(php *client) (string, map[string]any, string, error) {
				return "", nil, "per-backend random code: one-body-for-both can't express the " +
					"generate→accept exchange (covered by Go e2e test)", nil
			},
		},
		{
			// delete-connection: the DEPLOYED PHP 500s here ("There is no active
			// transaction" — an AntiCorruptionService defect, same class as budget
			// reset/revoke). Go performs it correctly. Not comparable against a
			// backend that errors; covered by the Go end-to-end test
			// (TestDeleteConnection_RemovesLink).
			name: "connection/delete-connection",
			build: func(php *client) (string, map[string]any, string, error) {
				return "", nil, "live PHP delete-connection 500s (AntiCorruptionService 'no active " +
					"transaction' defect); Go performs it correctly — not comparable", nil
			},
		},
	}
}
