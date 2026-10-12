// Package revisionguard replays the apiparity and mcpparity catalogues and
// fails any request that changes what get-account-transaction-list would
// return for an account without moving that account's transactions_revision.
// Clients re-download an account only when its revision moves, so a missed
// bump leaves their offline copy stale forever.
package revisionguard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/apiparity"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/mcpparity"
)

// Check runs every scenario of both catalogues on a fresh database from newDB,
// checking each request's effect on every account that exists before and
// after it. Accounts created or removed by the request are ignored, and a bump
// without a visible change is allowed.
func Check(t *testing.T, newDB func(testing.TB) *dbtest.DB) {
	t.Run("api", func(t *testing.T) {
		for _, sc := range apiparity.Catalogue() {
			t.Run(sc.Name, func(t *testing.T) {
				h, _ := watch(t, newDB(t))
				h.Replay(t, sc.Calls())
			})
		}
	})
	t.Run("mcp", func(t *testing.T) {
		for _, sc := range mcpparity.Catalogue() {
			t.Run(sc.Name, func(t *testing.T) {
				h, _ := watch(t, newDB(t))
				mcpparity.RunSteps(t, h, sc)
			})
		}
	})
	t.Run("guard", func(t *testing.T) {
		for _, sc := range scenarios() {
			t.Run(sc.Name, func(t *testing.T) {
				h, changed := watch(t, newDB(t))
				calls := sc.Calls()
				statuses, bodies := h.Replay(t, calls)
				for i, c := range calls {
					if statuses[i] < 200 || statuses[i] > 299 {
						t.Fatalf("[%s] expected 2xx, got %d: %s", c.Label, statuses[i], bodies[i])
					}
				}
				for _, label := range sc.Want {
					if !changed[label] {
						t.Errorf("%s changed no existing account's transactions; the scenario no longer exercises it", label)
					}
				}
			})
		}
	})
}

// watch installs the per-request check and returns the set of request labels
// that changed some pre-existing account's transaction data.
func watch(t *testing.T, db *dbtest.DB) (*apiparity.Harness, map[string]bool) {
	t.Helper()
	h := apiparity.NewHarness(t, db)
	changed := map[string]bool{}
	var before map[string]accountState
	h.Observe = func(t *testing.T, phase, label string) {
		t.Helper()
		switch phase {
		case "before":
			before = snapshot(t, db)
		case "after":
			after := snapshot(t, db)
			ids := make([]string, 0, len(after))
			for id := range after {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				b, ok := before[id]
				if !ok {
					continue
				}
				a := after[id]
				if a.fp == b.fp {
					continue
				}
				changed[label] = true
				if a.rev == b.rev {
					t.Errorf("%s: account %s transactions changed without a revision bump", label, id)
				}
			}
		}
	}
	return h, changed
}

type accountState struct {
	fp  string
	rev int64
}

// The columns are the ones get-account-transaction-list serializes, plus the
// author's name and avatar it returns in authors. A transfer appears under
// both of its accounts.
const fingerprintQuery = `
SELECT a.id, a.transactions_revision,
       t.id, t.user_id, t.account_id, t.account_recipient_id, t.category_id, t.payee_id, t.tag_id,
       t.description, t.spent_at, t.type, t.amount, t.amount_recipient, t.recurring_id,
       u.name, u.avatar
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id OR t.account_recipient_id = a.id
LEFT JOIN users u ON u.id = t.user_id
ORDER BY a.id, t.id`

const (
	colTxID            = 2
	colAmount          = 12
	colAmountRecipient = 13
	fingerprintCols    = 17
)

func snapshot(t *testing.T, db *dbtest.DB) map[string]accountState {
	t.Helper()
	labels := transactionLabels(t, db)
	imported := importedTransactions(t, db)

	rows, err := db.Raw.Query(fingerprintQuery)
	if err != nil {
		t.Fatalf("revisionguard: fingerprint query: %v", err)
	}
	defer rows.Close()

	hashes := map[string][]string{}
	revs := map[string]int64{}
	order := []string{}
	for rows.Next() {
		vals := make([]any, fingerprintCols)
		ptrs := make([]any, fingerprintCols)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("revisionguard: scan: %v", err)
		}
		accountID := text(vals[0])
		if _, seen := revs[accountID]; !seen {
			order = append(order, accountID)
			rev, err := toInt64(vals[1])
			if err != nil {
				t.Fatalf("revisionguard: account %s revision: %v", accountID, err)
			}
			revs[accountID] = rev
		}
		if vals[colTxID] == nil {
			continue
		}
		fields := make([]string, 0, fingerprintCols)
		for i := colTxID; i < fingerprintCols; i++ {
			switch {
			case vals[i] == nil:
				fields = append(fields, "<null>")
			case i == colAmount || i == colAmountRecipient:
				fields = append(fields, vo.NewDecimal(text(vals[i])).String())
			default:
				fields = append(fields, text(vals[i]))
			}
		}
		txID := text(vals[colTxID])
		fields = append(fields, "labels="+strings.Join(labels[txID], ","), fmt.Sprintf("imported=%t", imported[txID]))
		hashes[accountID] = append(hashes[accountID], strings.Join(fields, "\x1f"))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("revisionguard: rows: %v", err)
	}

	out := make(map[string]accountState, len(order))
	for _, id := range order {
		sum := sha256.Sum256([]byte(strings.Join(hashes[id], "\x1e")))
		out[id] = accountState{fp: hex.EncodeToString(sum[:]), rev: revs[id]}
	}
	return out
}

func transactionLabels(t *testing.T, db *dbtest.DB) map[string][]string {
	t.Helper()
	rows, err := db.Raw.Query(`SELECT transaction_id, label_id FROM transactions_labels ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("revisionguard: labels query: %v", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var txID, labelID string
		if err := rows.Scan(&txID, &labelID); err != nil {
			t.Fatalf("revisionguard: labels scan: %v", err)
		}
		out[txID] = append(out[txID], labelID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("revisionguard: labels rows: %v", err)
	}
	return out
}

func importedTransactions(t *testing.T, db *dbtest.DB) map[string]bool {
	t.Helper()
	rows, err := db.Raw.Query(`SELECT DISTINCT transaction_id FROM import_transaction_links WHERE transaction_id IS NOT NULL`)
	if err != nil {
		t.Fatalf("revisionguard: import links query: %v", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var txID string
		if err := rows.Scan(&txID); err != nil {
			t.Fatalf("revisionguard: import links scan: %v", err)
		}
		out[txID] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("revisionguard: import links rows: %v", err)
	}
	return out
}

func text(v any) string {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case string:
		return x
	case time.Time:
		return x.UTC().Format(time.DateTime)
	default:
		return fmt.Sprint(x)
	}
}

func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int32:
		return int64(x), nil
	case int:
		return int64(x), nil
	default:
		var n int64
		_, err := fmt.Sscan(text(v), &n)
		return n, err
	}
}
