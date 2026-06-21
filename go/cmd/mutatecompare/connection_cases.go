package main

// Connection mutation comparison cases.
//
// Scope reality check (HONEST): the live PHP backend the harness compares
// against runs the EconumoCloudBundle, which OVERRIDES four connection
// controllers with full implementations: generate-invite, accept-invite,
// delete-invite, delete-connection. The Go reimplementation targets the
// self-hosted EconumoBundle, where those same four are 501 stubs
// ("Not supported in Econumo One"). They therefore CANNOT be fairly compared —
// the two products implement different feature sets — and additionally rely on
// random invite codes / server-minted ids / a genuine two-user exchange that
// cannot be made deterministic across two independent backends. They are
// emitted as explicit [SKIP]s with that reason rather than forced into a false
// verdict.
//
// The two endpoints that ARE implemented in BOTH backends and share the same
// contract are set-account-access and revoke-account-access. Those are compared
// for real, with get-connection-list as the state read. The seed already holds
// a connection between the two seed users (kuznetsov2d <-> Irina) with many
// shared accounts in both directions, so these are single-user, deterministic
// mutations: the logged-in owner (kuznetsov2d) changes/revokes the OTHER user's
// (Irina's) access to an account kuznetsov2d owns.
//
// NOTE: the get-connection-list sharedAccounts array has a documented
// PHP-vs-Go ordering difference; the harness canonical compare sorts arrays, so
// only genuine content differences register.

const connectionList = "/api/v1/connection/get-connection-list"

// connectionUnsupportedMessage is the self-hosted EconumoBundle stub message Go
// returns (501). The live Cloud PHP implements these endpoints instead, so the
// skip reason names the mismatch explicitly.
const cloudOnlyReason = "cloud-only: live PHP runs EconumoCloudBundle (implemented); " +
	"Go targets self-hosted EconumoBundle (501 stub) — not fairly comparable, and " +
	"depends on random invite code / two-user exchange (non-deterministic)"

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
			name: "connection/generate-invite",
			build: func(php *client) (string, map[string]any, string, error) {
				return "", nil, cloudOnlyReason, nil
			},
		},
		{
			name: "connection/accept-invite",
			build: func(php *client) (string, map[string]any, string, error) {
				return "", nil, cloudOnlyReason, nil
			},
		},
		{
			name: "connection/delete-invite",
			build: func(php *client) (string, map[string]any, string, error) {
				return "", nil, cloudOnlyReason, nil
			},
		},
		{
			name: "connection/delete-connection",
			build: func(php *client) (string, map[string]any, string, error) {
				return "", nil, cloudOnlyReason, nil
			},
		},
	}
}
