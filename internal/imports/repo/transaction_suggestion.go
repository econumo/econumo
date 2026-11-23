package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// InsertTransactionSuggestions keeps the first row stored for a link: a later
// job never rewrites a suggestion the user may already have seen or filled.
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

func (r *Repo) ListOpenTransactionSuggestionsByUser(ctx context.Context, userID vo.Id) ([]model.ImportTransactionSuggestion, error) {
	rows, err := r.q.ListOpenImportTransactionSuggestionsByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionSuggestion, 0, len(rows))
	for _, row := range rows {
		s, err := transactionSuggestionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *Repo) LockTransactionSuggestion(ctx context.Context, linkID vo.Id) (*model.ImportTransactionSuggestion, error) {
	n, err := r.q.LockImportTransactionSuggestion(ctx, r.db(ctx), linkID.String())
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errs.NewNotFound("Import suggestion not found")
	}
	row, err := r.q.GetImportTransactionSuggestion(ctx, r.db(ctx), linkID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import suggestion not found")
		}
		return nil, err
	}
	return transactionSuggestionFromRow(row)
}

func (r *Repo) MarkTransactionSuggestionFilled(ctx context.Context, linkID vo.Id, now time.Time) error {
	return r.q.MarkImportTransactionSuggestionFilled(ctx, r.db(ctx), markTxSuggestionFilledPs{UpdatedAt: now, LinkID: linkID.String()})
}

func (r *Repo) DeleteTransactionSuggestion(ctx context.Context, linkID vo.Id) error {
	return r.q.DeleteImportTransactionSuggestion(ctx, r.db(ctx), linkID.String())
}

func (r *Repo) DeleteOpenTransactionSuggestionsByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteOpenImportTransactionSuggestionsByUser(ctx, r.db(ctx), userID.String())
}

// DeleteTransactionSuggestionsByLinks and FilledTransactionSuggestionLinks take
// a variadic IN list, which sqlc cannot express on both engines.
func (r *Repo) DeleteTransactionSuggestionsByLinks(ctx context.Context, linkIDs []vo.Id) error {
	if len(linkIDs) == 0 {
		return nil
	}
	query, args := r.inList("DELETE FROM import_transaction_suggestions WHERE link_id IN (", linkIDs)
	_, err := r.db(ctx).ExecContext(ctx, query, args...)
	return err
}

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

func transactionSuggestionFromRow(row txSuggestionRow) (*model.ImportTransactionSuggestion, error) {
	linkID, err := vo.ParseId(row.LinkID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	s := &model.ImportTransactionSuggestion{
		LinkID: linkID, UserID: userID, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if s.CategoryID, err = parseOptionalID(row.CategoryID); err != nil {
		return nil, err
	}
	if s.PayeeID, err = parseOptionalID(row.PayeeID); err != nil {
		return nil, err
	}
	switch row.TransactionType {
	case model.TransactionTypeExpense.Alias():
		s.Type = model.TransactionTypeExpense
	case model.TransactionTypeIncome.Alias():
		s.Type = model.TransactionTypeIncome
	default:
		return nil, fmt.Errorf("import transaction suggestion %s: unknown transaction type", row.LinkID)
	}
	return s, nil
}
