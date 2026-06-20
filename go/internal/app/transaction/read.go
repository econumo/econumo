// Read side: get-transaction-list and export-transaction-list.
package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
)

// GetTransactionList returns transactions for: a single account (if accountId
// given, access-checked), or a [periodStart, periodEnd) window across the user's
// visible accounts, or all visible-account transactions. Matches the PHP
// TransactionListService.getTransactionList branching.
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req GetTransactionListRequest) (*GetTransactionListResult, error) {
	var txs []*domtransaction.Transaction

	switch {
	case req.AccountId != "":
		accountID, err := vo.ParseId(req.AccountId)
		if err != nil {
			return nil, err
		}
		if aerr := s.checkAccountOwned(ctx, userID, accountID); aerr != nil {
			return nil, aerr
		}
		list, err := s.repo.ListByAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		txs = list
	default:
		ids, err := s.visible.VisibleAccountIDs(ctx, userID)
		if err != nil {
			return nil, err
		}
		var start, end time.Time
		if req.PeriodStart != "" && req.PeriodEnd != "" {
			if start, err = parseFlexible(req.PeriodStart); err != nil {
				return nil, err
			}
			if end, err = parseFlexible(req.PeriodEnd); err != nil {
				return nil, err
			}
		}
		list, err := s.repo.ListByAccountIDs(ctx, ids, start, end)
		if err != nil {
			return nil, err
		}
		txs = list
	}

	items := make([]TransactionResult, 0, len(txs))
	for _, t := range txs {
		r, err := s.toResult(ctx, t)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return &GetTransactionListResult{Items: items}, nil
}

// parseFlexible parses a period bound, accepting both "Y-m-d H:i:s" and
// "Y-m-d" (the frontend sends either; PHP's DateTimeImmutable is lenient).
func parseFlexible(v string) (time.Time, error) {
	if t, err := time.Parse(apiDatetimeLayout, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}
