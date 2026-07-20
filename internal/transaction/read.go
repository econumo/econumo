package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetTransactionList returns transactions for: a single account (if accountId
// given, access-checked, optionally narrowed to a [periodStart, periodEnd)
// window when BOTH bounds are set — a lone bound is ignored), or a
// [periodStart, periodEnd) window across the user's visible accounts, or all
// visible-account transactions. The four classification filter fields
// (Uncategorized/CategoryId/PayeeId/TagId) are MCP-only — REST never sets
// them, so the zero-filter path below must stay byte-identical to the
// pre-filter behavior: the single-account-no-period case keeps calling
// ListByAccount exactly as before, only routing through the filter-aware
// ListByAccountIDs when a filter is actually supplied.
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req model.TransactionListRequest) (*model.GetTransactionListResult, error) {
	filter, hasFilter, err := buildFilter(req)
	if err != nil {
		return nil, err
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
		if req.PeriodStart != "" && req.PeriodEnd != "" {
			start, perr := parseFlexible(req.PeriodStart)
			if perr != nil {
				return nil, perr
			}
			end, perr := parseFlexible(req.PeriodEnd)
			if perr != nil {
				return nil, perr
			}
			list, lerr := s.repo.ListByAccountIDs(ctx, []vo.Id{accountID}, start, end, filter)
			if lerr != nil {
				return nil, lerr
			}
			txs = list
			break
		}
		if hasFilter {
			list, lerr := s.repo.ListByAccountIDs(ctx, []vo.Id{accountID}, time.Time{}, time.Time{}, filter)
			if lerr != nil {
				return nil, lerr
			}
			txs = list
			break
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
		list, err := s.repo.ListByAccountIDs(ctx, ids, start, end, filter)
		if err != nil {
			return nil, err
		}
		txs = list
	}

	// Resolve each transaction's author embed through a per-request cache. A list
	// can contain thousands of rows that nearly all share the same author (the
	// owner, plus a few connected users on shared accounts), and each GetOwner is
	// a DB round-trip (user row + options). Without this cache that is an N+1 that
	// dominates the endpoint's latency.
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
	return &model.GetTransactionListResult{Items: items}, nil
}

// buildFilter parses the request's classification filter fields (MCP-only;
// REST leaves them all zero) into a model.TransactionFilter, plus whether any
// filter was actually supplied. uncategorized and categoryId are rejected
// together (they contradict: uncategorized means categoryId IS NULL). Ids are
// parsed independently of TransactionListRequest.Validate() since MCP tools
// call this service directly and never run Validate() (see
// internal/web/mcp) — this is the tier-2 pass that actually enforces UUID
// shape for MCP callers, mirroring how accountId is parsed below regardless
// of Validate() having run.
func buildFilter(req model.TransactionListRequest) (model.TransactionFilter, bool, error) {
	if req.Uncategorized && req.CategoryId != "" {
		return model.TransactionFilter{}, false, errs.NewValidation("uncategorized and categoryId cannot both be set")
	}
	var f model.TransactionFilter
	f.Uncategorized = req.Uncategorized
	if req.CategoryId != "" {
		id, err := vo.ParseId(req.CategoryId)
		if err != nil {
			return model.TransactionFilter{}, false, err
		}
		f.CategoryID = &id
	}
	if req.PayeeId != "" {
		id, err := vo.ParseId(req.PayeeId)
		if err != nil {
			return model.TransactionFilter{}, false, err
		}
		f.PayeeID = &id
	}
	if req.TagId != "" {
		id, err := vo.ParseId(req.TagId)
		if err != nil {
			return model.TransactionFilter{}, false, err
		}
		f.TagID = &id
	}
	hasFilter := f.Uncategorized || f.CategoryID != nil || f.PayeeID != nil || f.TagID != nil
	return f, hasFilter, nil
}

// parseFlexible parses a period bound, accepting both "Y-m-d H:i:s" and "Y-m-d"
// (the frontend sends either).
func parseFlexible(v string) (time.Time, error) {
	if t, err := time.Parse(datetime.Layout, v); err == nil {
		return t, nil
	}
	return time.Parse(datetime.DateLayout, v)
}
