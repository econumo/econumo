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
		if aerr := s.checkViewAccess(ctx, userID, accountID); aerr != nil {
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

	// Resolve each transaction's author embed through a per-request cache. A list
	// can contain thousands of rows that nearly all share the same author (the
	// owner, plus a few connected users on shared accounts), and each GetOwner is
	// a DB round-trip (user row + options). Without this cache that is an N+1 that
	// dominated the endpoint's latency; PHP avoids it via Doctrine's identity map.
	authors := make(map[string]AuthorResult)
	items := make([]TransactionResult, 0, len(txs))
	for _, t := range txs {
		uid := t.UserId().String()
		author, ok := authors[uid]
		if !ok {
			av, err := s.users.GetOwner(ctx, uid)
			if err != nil {
				return nil, err
			}
			author = AuthorResult{Id: av.ID, Avatar: av.Avatar, Name: av.Name}
			authors[uid] = author
		}
		items = append(items, s.buildResult(t, author))
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
