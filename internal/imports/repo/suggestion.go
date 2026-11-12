package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (r *Repo) InsertSuggestion(ctx context.Context, s *model.ImportSuggestion) error {
	err := r.q.InsertImportSuggestion(ctx, r.db(ctx), insertSuggestionParams{
		ID: s.ID.String(), UserID: s.UserID.String(), MatchField: s.MatchField, MatchText: s.MatchText,
		CategoryID: optionalID(s.CategoryID), PayeeID: optionalID(s.PayeeID), TransactionType: s.Type.Alias(), Status: s.Status,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	})
	if err != nil {
		return err
	}
	return r.insertSuggestionLinks(ctx, s.ID, s.LinkIDs)
}

// AddSuggestionLinks answers false, writing nothing, when the suggestion is no
// longer open. The touch takes the row lock first, so on PostgreSQL a delete
// racing it either waits for this transaction or has already made it a miss,
// never a foreign-key failure on the link insert.
func (r *Repo) AddSuggestionLinks(ctx context.Context, suggestionID vo.Id, linkIDs []vo.Id, now time.Time) (bool, error) {
	n, err := r.q.TouchOpenImportSuggestion(ctx, r.db(ctx), touchSuggestionParams{UpdatedAt: now, ID: suggestionID.String()})
	if err != nil || n == 0 {
		return false, err
	}
	return true, r.insertSuggestionLinks(ctx, suggestionID, linkIDs)
}

func (r *Repo) insertSuggestionLinks(ctx context.Context, suggestionID vo.Id, linkIDs []vo.Id) error {
	for _, id := range linkIDs {
		if err := r.q.InsertImportSuggestionLink(ctx, r.db(ctx), insertSuggestionLinkPs{SuggestionID: suggestionID.String(), LinkID: id.String()}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) GetSuggestion(ctx context.Context, id vo.Id) (*model.ImportSuggestion, error) {
	row, err := r.q.GetImportSuggestionByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import suggestion not found")
		}
		return nil, err
	}
	s, err := suggestionFromRow(row)
	if err != nil {
		return nil, err
	}
	links, err := r.q.ListImportSuggestionLinks(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		lid, err := vo.ParseId(l.LinkID)
		if err != nil {
			return nil, err
		}
		s.LinkIDs = append(s.LinkIDs, lid)
	}
	return s, nil
}

func (r *Repo) ListOpenSuggestionsByUser(ctx context.Context, userID vo.Id) ([]model.ImportSuggestion, error) {
	rows, err := r.q.ListOpenImportSuggestionsByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	linkRows, err := r.q.ListOpenImportSuggestionLinksByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	links := map[string][]vo.Id{}
	for _, l := range linkRows {
		lid, err := vo.ParseId(l.LinkID)
		if err != nil {
			return nil, err
		}
		links[l.SuggestionID] = append(links[l.SuggestionID], lid)
	}
	out := make([]model.ImportSuggestion, 0, len(rows))
	for _, row := range rows {
		s, err := suggestionFromRow(row)
		if err != nil {
			return nil, err
		}
		s.LinkIDs = links[row.ID]
		out = append(out, *s)
	}
	return out, nil
}

func (r *Repo) DeleteSuggestion(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportSuggestion(ctx, r.db(ctx), id.String())
}

func suggestionFromRow(row suggestionRow) (*model.ImportSuggestion, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	s := &model.ImportSuggestion{
		ID: id, UserID: userID, MatchField: row.MatchField, MatchText: row.MatchText,
		Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
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
		return nil, fmt.Errorf("import suggestion %s: unknown transaction type", row.ID)
	}
	return s, nil
}
