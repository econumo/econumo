package transaction

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetTransactionList returns transactions in one of four modes: a keyset page
// for one account (accountId+limit[+cursor]), the newest perAccountLimit rows
// per visible account (perAccountLimit), a single account's full list
// (accountId), or a [periodStart, periodEnd) window across visible accounts /
// everything visible (legacy).
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req model.TransactionListRequest) (*model.GetTransactionListResult, error) {
	switch {
	case req.PerAccountLimit != "":
		return s.listBootWindows(ctx, userID, req.PerAccountLimitValue())
	case req.Limit != "":
		return s.listPage(ctx, userID, req)
	}

	var txs []*model.Transaction
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

	items, err := s.buildItems(ctx, txs)
	if err != nil {
		return nil, err
	}
	return &model.GetTransactionListResult{Items: items}, nil
}

func (s *Service) listPage(ctx context.Context, userID vo.Id, req model.TransactionListRequest) (*model.GetTransactionListResult, error) {
	accountID, err := vo.ParseId(req.AccountId)
	if err != nil {
		return nil, err
	}
	if aerr := s.checkViewAccess(ctx, userID, accountID); aerr != nil {
		return nil, aerr
	}
	var after *PageCursor
	if req.Cursor != "" {
		c, cerr := decodeCursor(req.Cursor)
		if cerr != nil {
			return nil, cerr
		}
		after = &c
	}
	limit := req.LimitValue()
	rows, err := s.repo.ListPageByAccount(ctx, accountID, after, limit+1)
	if err != nil {
		return nil, err
	}
	page := &model.TransactionPageResult{HasMore: len(rows) > limit}
	if page.HasMore {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := EncodeCursor(PageCursor{SpentAt: last.SpentAt, ID: last.ID})
		page.NextCursor = &c
	}
	items, err := s.buildItems(ctx, rows)
	if err != nil {
		return nil, err
	}
	return &model.GetTransactionListResult{Items: items, Page: page}, nil
}

func (s *Service) listBootWindows(ctx context.Context, userID vo.Id, limit int) (*model.GetTransactionListResult, error) {
	ids, err := s.visible.VisibleAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	windows, err := s.repo.ListRecentByAccountIDs(ctx, ids, limit+1)
	if err != nil {
		return nil, err
	}
	accounts := make([]model.TransactionAccountPageResult, 0, len(ids))
	seen := make(map[string]bool)
	var txs []*model.Transaction
	for _, id := range ids { // VisibleAccountIDs order keeps the accounts block deterministic
		rows := windows[id.String()]
		info := model.TransactionAccountPageResult{Id: id.String(), HasMore: len(rows) > limit}
		if info.HasMore {
			rows = rows[:limit]
			last := rows[len(rows)-1]
			c := EncodeCursor(PageCursor{SpentAt: last.SpentAt, ID: last.ID})
			info.NextCursor = &c
		}
		accounts = append(accounts, info)
		for _, t := range rows {
			if !seen[t.ID.String()] {
				seen[t.ID.String()] = true
				txs = append(txs, t)
			}
		}
	}
	sort.Slice(txs, func(i, j int) bool {
		if !txs[i].SpentAt.Equal(txs[j].SpentAt) {
			return txs[i].SpentAt.After(txs[j].SpentAt)
		}
		return txs[i].ID.String() < txs[j].ID.String()
	})
	items, err := s.buildItems(ctx, txs)
	if err != nil {
		return nil, err
	}
	return &model.GetTransactionListResult{Items: items, Accounts: accounts}, nil
}

// buildItems resolves each transaction's author embed through a per-request
// cache. A list can contain thousands of rows that nearly all share the same
// author (the owner, plus a few connected users on shared accounts), and each
// GetOwner is a DB round-trip; without the cache that is an N+1 that dominates
// the endpoint's latency.
func (s *Service) buildItems(ctx context.Context, txs []*model.Transaction) ([]model.TransactionResult, error) {
	authors := make(map[string]model.UserResult)
	items := make([]model.TransactionResult, 0, len(txs))
	for _, t := range txs {
		uid := t.UserID.String()
		author, ok := authors[uid]
		if !ok {
			av, err := s.users.GetOwner(ctx, uid)
			if err != nil {
				return nil, err
			}
			author = model.UserResult{Id: av.ID, Avatar: av.Avatar, Name: av.Name}
			authors[uid] = author
		}
		items = append(items, s.buildResult(t, author))
	}
	return items, nil
}

// parseFlexible parses a period bound, accepting both "Y-m-d H:i:s" and "Y-m-d"
// (the frontend sends either).
func parseFlexible(v string) (time.Time, error) {
	if t, err := time.Parse(datetime.Layout, v); err == nil {
		return t, nil
	}
	return time.Parse(datetime.DateLayout, v)
}
