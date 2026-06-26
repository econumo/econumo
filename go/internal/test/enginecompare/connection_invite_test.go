//go:build enginecompare

package enginecompare

// Full invite -> accept -> delete-connection flow exercised against BOTH engines
// through the real production handler.
//
// The byte-parity catalogue can't cover this: accept-invite redeems a RANDOM
// code, so responses differ between engines by construction. Instead this
// asserts the write flow WORKS end-to-end on each engine — generate-invite
// upserts a code, accept-invite redeems it and writes the bidirectional
// users_connections rows, and delete-connection removes them. It mirrors the
// per-engine password-reset test: the repo layer is engine-compared elsewhere,
// but this nails the full HTTP stack on the PostgreSQL adapter production runs.
//
// Synthetic data only: dbtest provisions a throwaway, randomly-named database.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// carol is a third user, seeded unconnected, who redeems the owner's invite.
const (
	apiCarolID    = "33333333-3333-3333-3333-333333333333"
	apiCarolEmail = "carol@example.test"
)

func TestConnectionInviteFlow_PerEngine(t *testing.T) {
	run := func(t *testing.T, db *dbtest.DB) {
		h := newAPIHarness(t, db)

		// Seed a third, unconnected user to redeem the invite (owner+guest are
		// already connected by the shared fixture).
		fixture.New(t, db).WithCrypto(apiDataSalt).User(fixture.User{ID: apiCarolID, Email: apiCarolEmail, Name: "Carol"})
		fixture.New(t, db).DefaultOptions(apiCarolID)

		ownerTok := h.token(t, apiOwnerID, apiOwnerEmail)
		carolTok := h.token(t, apiCarolID, apiCarolEmail)

		// 1. Owner generates an invite; capture the issued code.
		st, body := h.call(t, http.MethodPost, "/api/v1/connection/generate-invite", ownerTok, map[string]any{})
		if st != http.StatusOK {
			t.Fatalf("generate-invite = %d; body: %s", st, body)
		}
		code := extractInviteCode(t, body)
		if len(code) != 5 {
			t.Fatalf("invite code = %q, want 5 chars", code)
		}

		// 2. Carol redeems it -> 200, and the response lists her new connection.
		st, body = h.call(t, http.MethodPost, "/api/v1/connection/accept-invite", carolTok, map[string]any{"code": code})
		if st != http.StatusOK {
			t.Fatalf("accept-invite = %d; body: %s", st, body)
		}

		// 3. The bidirectional link exists in users_connections (both rows).
		if c := countConnection(t, db, apiOwnerID, apiCarolID); c != 1 {
			t.Errorf("owner->carol link = %d, want 1", c)
		}
		if c := countConnection(t, db, apiCarolID, apiOwnerID); c != 1 {
			t.Errorf("carol->owner link = %d, want 1", c)
		}

		// 4. Owner deletes the connection -> both rows are removed.
		st, body = h.call(t, http.MethodPost, "/api/v1/connection/delete-connection", ownerTok, map[string]any{"id": apiCarolID})
		if st != http.StatusOK {
			t.Fatalf("delete-connection = %d; body: %s", st, body)
		}
		if c := countConnection(t, db, apiOwnerID, apiCarolID) + countConnection(t, db, apiCarolID, apiOwnerID); c != 0 {
			t.Errorf("connection links after delete = %d, want 0", c)
		}

		// 5. Redeeming the (now-cleared) code again fails — single use.
		st, _ = h.call(t, http.MethodPost, "/api/v1/connection/accept-invite", carolTok, map[string]any{"code": code})
		if st == http.StatusOK {
			t.Fatal("re-accepting a consumed invite code should not succeed")
		}
	}

	t.Run("sqlite", func(t *testing.T) { run(t, dbtest.NewSQLite(t)) })
	t.Run("postgresql", func(t *testing.T) { run(t, dbtest.NewPostgres(t)) }) // SKIPs if env unset
}

// extractInviteCode pulls data.item.code out of a generate-invite envelope.
func extractInviteCode(t *testing.T, raw []byte) string {
	t.Helper()
	var env struct {
		Data struct {
			Item struct {
				Code string `json:"code"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode generate-invite envelope: %v\nbody: %s", err, raw)
	}
	return env.Data.Item.Code
}

// countConnection returns how many users_connections rows link userID -> connectedID.
func countConnection(t *testing.T, db *dbtest.DB, userID, connectedID string) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM users_connections WHERE user_id = " + placeholder(db, 1) + " AND connected_user_id = " + placeholder(db, 2)
	if err := db.Raw.QueryRow(q, userID, connectedID).Scan(&n); err != nil {
		t.Fatalf("count connection (%s): %v", db.Engine, err)
	}
	return n
}
