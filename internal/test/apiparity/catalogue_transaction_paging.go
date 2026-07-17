package apiparity

// Paging scenarios: boot mode (perAccountLimit), keyset page mode
// (accountId+limit[+cursor]), and the paging validation errors. The cursor is
// built from fixture constants so the golden stays deterministic.
//
// The fixture builder stamps each seeded row with a strictly increasing
// timestamp (internal/test/fixture.Builder.now, 1-second step from a fixed
// 2024-04-01T12:00:00Z base), and Txn2 is seeded right after Txn1 with no
// intervening rows, so Txn2.spent_at (12:00:18Z) is exactly one second after
// Txn1.spent_at (12:00:17Z): Txn2 is the newest row on OwnerAccount and sorts
// first in (spent_at DESC, id ASC) order. Confirmed by querying a seeded DB
// directly rather than assumed.
import (
	"net/url"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/transaction"
)

// transactionPagingTxn2SpentAt is Txn2's seeded spent_at (see comment above).
var transactionPagingTxn2SpentAt = time.Date(2024, 4, 1, 12, 0, 18, 0, time.UTC)

func init() {
	register(Scenario{Name: "transaction_paging", Calls: func() []Call {
		cursor := domtransaction.EncodeCursor(domtransaction.PageCursor{
			SpentAt: transactionPagingTxn2SpentAt, ID: vo.MustParseId(Txn2),
		})
		return []Call{
			{Label: "boot-window", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?perAccountLimit=1", Auth: "owner"},
			{Label: "page-first", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount + "&limit=1", Auth: "owner"},
			{Label: "page-after", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount + "&limit=1&cursor=" + url.QueryEscape(cursor), Auth: "owner"},
			{Label: "err:paging-conflict", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?perAccountLimit=1&accountId=" + OwnerAccount, Auth: "owner"},
			{Label: "err:limit-without-account", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?limit=5", Auth: "owner"},
			{Label: "err:bad-cursor", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount + "&limit=1&cursor=@@@", Auth: "owner"},
		}
	}})
}
