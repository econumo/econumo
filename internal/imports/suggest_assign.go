package imports

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// assignment is what the AI job writes onto one row's empty fields.
type assignment struct {
	row        suggestRow
	categoryID *vo.Id
	payeeID    *vo.Id
	// payeeName is a payee to create, or to reuse when this run already
	// created it, when payeeID is nil.
	payeeName string
}

// assign writes each row's values into the fields its transaction still has
// empty, one savepoint per row: a row that fails is skipped with a WARN and
// the others proceed. Rows of one merchant share the payee created for it.
// It returns the payees created, by similarityText of their names.
func (s *Service) assign(ctx context.Context, owner vo.Id, list []assignment) map[string]vo.Id {
	created := map[string]vo.Id{}
	now := s.clk.Now().UTC()
	for _, a := range list {
		var newPayee *vo.Id
		err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			var err error
			newPayee, err = s.assignRow(ctx, owner, a, created, now)
			return err
		})
		if err != nil {
			// Never the error text: a payee name can be in it.
			slog.Warn("import assign row skipped", "user_id", owner.String(), "link_id", a.row.Link.ID.String(), "err_type", fmt.Sprintf("%T", err))
			continue
		}
		// only once the row's savepoint is released: a rolled-back payee must
		// not be handed to the next row
		if newPayee != nil {
			created[similarityText(a.payeeName)] = *newPayee
		}
	}
	return created
}

// assignRow locks the transaction and reads it and its link again, so a hand
// edit since the scan is never overwritten: only fields still empty are
// written, and a row gone, moved off the owner's account or of another type
// by now is left alone. The applied snapshot moves for the fields written,
// as a rule's write does, so a later apply-rule may replace them.
func (s *Service) assignRow(ctx context.Context, owner vo.Id, a assignment, created map[string]vo.Id, now time.Time) (*vo.Id, error) {
	if a.row.Link.TransactionID == nil {
		return nil, nil
	}
	txID := *a.row.Link.TransactionID
	if err := s.repo.LockTransactionRow(ctx, txID); err != nil {
		return nil, err
	}
	l, live, err := s.suggestionTarget(ctx, owner, a.row.Link.ID)
	if err != nil || live == nil || live.ID != txID || live.Type != a.row.Type {
		return nil, err
	}
	fillCategory := live.CategoryID == nil && a.categoryID != nil
	payeeID := a.payeeID
	if live.PayeeID != nil {
		payeeID = nil
	}
	var newPayee *vo.Id
	if live.PayeeID == nil && payeeID == nil && a.payeeName != "" {
		if id, ok := created[similarityText(a.payeeName)]; ok {
			payeeID = &id
		} else if s.payees != nil {
			p, err := s.payees.CreatePayee(ctx, owner, a.payeeName)
			if err != nil {
				return nil, err
			}
			id, err := vo.ParseId(p.ID)
			if err != nil {
				return nil, err
			}
			payeeID, newPayee = &id, &id
		}
	}
	if !fillCategory && payeeID == nil {
		return nil, nil
	}
	next := model.ImportClassification{CategoryID: live.CategoryID, PayeeID: live.PayeeID, TagID: live.TagID, LabelIDs: live.LabelIDs}
	record := model.ImportTransactionSuggestion{
		LinkID: l.ID, UserID: owner, Type: live.Type,
		Status: model.ImportTransactionSuggestionStatusFilled, CreatedAt: now, UpdatedAt: now,
	}
	if fillCategory {
		next.CategoryID, record.CategoryID, l.AppliedCategoryID = a.categoryID, a.categoryID, a.categoryID
	}
	if payeeID != nil {
		next.PayeeID, record.PayeeID, l.AppliedPayeeID = payeeID, payeeID, payeeID
	}
	if _, err := s.txns.UpdateTransaction(ctx, owner, updateRequest(live, next)); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateLink(ctx, l); err != nil {
		return nil, err
	}
	if err := s.repo.InsertTransactionSuggestions(ctx, []model.ImportTransactionSuggestion{record}); err != nil {
		return nil, err
	}
	return newPayee, nil
}

// openAssignments gives each row joined to an open suggestion before the AI
// call that suggestion's values, only for the fields the row lacks. A
// suggestion found gone covers nothing: its rows go to the AI.
func openAssignments(joins map[vo.Id][]vo.Id, gone []vo.Id, open []model.ImportSuggestion, rows []suggestRow) []assignment {
	goneSet := make(map[vo.Id]bool, len(gone))
	for _, id := range gone {
		goneSet[id] = true
	}
	byID := make(map[vo.Id]model.ImportSuggestion, len(open))
	for _, o := range open {
		byID[o.ID] = o
	}
	joinedTo := map[vo.Id]model.ImportSuggestion{}
	for id, links := range joins {
		o, ok := byID[id]
		if !ok || goneSet[id] {
			continue
		}
		for _, l := range links {
			joinedTo[l] = o
		}
	}
	var out []assignment
	for _, r := range rows {
		o, ok := joinedTo[r.Link.ID]
		if !ok {
			continue
		}
		a := assignment{row: r}
		if r.Missing.Category {
			a.categoryID = o.CategoryID
		}
		if r.Missing.Payee {
			a.payeeID = o.PayeeID
			if o.PayeeID == nil && o.PayeeName != nil {
				a.payeeName = *o.PayeeName
			}
		}
		if a.categoryID != nil || a.payeeID != nil || a.payeeName != "" {
			out = append(out, a)
		}
	}
	return out
}
