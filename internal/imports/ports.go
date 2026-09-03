// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package imports

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountReader is the slice of the account feature the pipeline needs:
// ownership (link-account), dormancy (a deleted account queues instead of
// failing), and the currency to convert into.
type AccountReader interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	AccountDeleted(ctx context.Context, accountID vo.Id) (bool, error)
	AccountCurrencyCode(ctx context.Context, accountID vo.Id) (string, error)
}

// CurrencyConverter re-denominates amount (decimal text) from code `from`
// into `to` at `at`. ok=false means "no rate for that pair and date" (or an
// unknown code) — the event queues; the shared convertor's silent 1:1
// fallback must never reach an imported amount.
type CurrencyConverter interface {
	Convert(ctx context.Context, userID vo.Id, from, to, amount string, at time.Time) (converted string, ok bool, err error)
}

// TransactionCreator is the transaction feature's create use case, so an
// import goes through exactly the checks a hand-entered transaction does
// (deleted account, write access, idempotency on the request id).
type TransactionCreator interface {
	CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
}

// TransactionLister pre-selects the matcher's candidates: the account's
// transactions with spent_at inside [from, to].
type TransactionLister interface {
	ListByAccount(ctx context.Context, accountID vo.Id, from, to time.Time) ([]*model.Transaction, error)
}

// AttemptLimiter caps ingest per user; every request counts (Allow then
// Fail). A nil limiter disables protection (tests).
type AttemptLimiter interface {
	Allow(scope, key string) error
	Fail(scope, key string)
}

const RateScopeIngest = "ingest"
