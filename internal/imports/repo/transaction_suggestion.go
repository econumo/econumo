package repo

import (
	"context"
	"strconv"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// InsertTransactionSuggestions keeps the first row stored for a link: a later
// job never rewrites the record of what an earlier one assigned.
func (r *Repo) InsertTransactionSuggestions(ctx context.Context, rows []model.ImportTransactionSuggestion) error {
	for _, s := range rows {
		err := r.q.InsertImportTransactionSuggestion(ctx, r.db(ctx), insertTxSuggestionParams{
			LinkID: s.LinkID.String(), UserID: s.UserID.String(),
			CategoryID: optionalID(s.CategoryID), PayeeID: optionalID(s.PayeeID),
			TransactionType: s.Type.Alias(), Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) LockTransactionRow(ctx context.Context, transactionID vo.Id) error {
	return r.q.LockImportTransactionRow(ctx, r.db(ctx), transactionID.String())
}

func (r *Repo) DeleteOpenTransactionSuggestionsByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteOpenImportTransactionSuggestionsByUser(ctx, r.db(ctx), userID.String())
}

// FilledTransactionSuggestionLinks takes a variadic IN list, which sqlc cannot
// express on both engines.
func (r *Repo) FilledTransactionSuggestionLinks(ctx context.Context, linkIDs []vo.Id) (map[vo.Id]bool, error) {
	out := map[vo.Id]bool{}
	if len(linkIDs) == 0 {
		return out, nil
	}
	query, args := r.inList("SELECT link_id FROM import_transaction_suggestions WHERE status = 'filled' AND link_id IN (", linkIDs)
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		id, err := vo.ParseId(raw)
		if err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *Repo) inList(prefix string, ids []vo.Id) (string, []any) {
	var sb strings.Builder
	sb.WriteString(prefix)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			sb.WriteByte(',')
		}
		if r.driver == "postgresql" {
			sb.WriteString("$" + strconv.Itoa(i+1))
		} else {
			sb.WriteByte('?')
		}
		args[i] = id.String()
	}
	sb.WriteByte(')')
	return sb.String(), args
}
