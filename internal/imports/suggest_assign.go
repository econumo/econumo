package imports

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
// It returns the payees created, by similarityText of their names, and the
// links of the rows it wrote.
func (s *Service) assign(ctx context.Context, owner vo.Id, list []assignment) (map[string]vo.Id, map[vo.Id]bool, error) {
	created := map[string]vo.Id{}
	written := map[vo.Id]bool{}
	now := s.clk.Now().UTC()
	s.lockAssignRows(ctx, owner, list)
	// The locks can take a while: a category or payee archived meanwhile is
	// not written, and neither is a new payee whose name an archived one took.
	vocab, err := s.suggestVocabulary(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range list {
		a = vocab.current(a)
		var newPayee *vo.Id
		var wrote bool
		err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			var err error
			newPayee, wrote, err = s.assignRow(ctx, owner, a, created, now)
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
		if wrote {
			written[a.row.Link.ID] = true
		}
	}
	return created, written, nil
}

// current keeps only the values still active, and turns a new-payee name into
// the payee that carries it by now (none when only an archived one does).
func (v suggestVocabulary) current(a assignment) assignment {
	if a.categoryID != nil && !v.active[*a.categoryID] {
		a.categoryID = nil
	}
	if a.payeeID != nil && !v.active[*a.payeeID] {
		a.payeeID = nil
	}
	if a.payeeID == nil && a.payeeName != "" {
		if id, isNew := v.names.resolve(a.payeeName); id != nil {
			a.payeeID, a.payeeName = id, ""
		} else if !isNew {
			a.payeeName = ""
		}
	}
	return a
}

// lockAssignRows takes every row lock of the run, in id order, before the first
// write. Each write bumps its account's revision (locking the account row);
// waiting for a later row's lock while holding the account would deadlock with a
// hand edit that holds that row and waits for the account. A lock that fails is
// left to the row's own savepoint, which skips the row.
func (s *Service) lockAssignRows(ctx context.Context, owner vo.Id, list []assignment) {
	seen := map[vo.Id]bool{}
	var ids []vo.Id
	for _, a := range list {
		if id := a.row.Link.TransactionID; id != nil && !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.LockTransactionRow(ctx, id) })
		if err != nil {
			slog.Warn("import assign lock failed", "user_id", owner.String(), "transaction_id", id.String(), "err_type", fmt.Sprintf("%T", err))
		}
	}
}

// assignRow locks the transaction and reads it and its link again, so a hand
// edit since the scan is never overwritten: only fields still empty are
// written, and a row gone, moved off the owner's account, of another type or
// with a category by now is left alone: a transaction a rule or the user
// categorised keeps its empty payee (the owner's call). The applied snapshot
// moves for the fields written, as a rule's write does, so a later apply-rule
// may replace them.
func (s *Service) assignRow(ctx context.Context, owner vo.Id, a assignment, created map[string]vo.Id, now time.Time) (*vo.Id, bool, error) {
	if a.row.Link.TransactionID == nil {
		return nil, false, nil
	}
	txID := *a.row.Link.TransactionID
	if err := s.repo.LockTransactionRow(ctx, txID); err != nil {
		return nil, false, err
	}
	l, live, err := s.suggestionTarget(ctx, owner, a.row.Link.ID)
	if err != nil || live == nil || live.ID != txID || live.Type != a.row.Type || live.CategoryID != nil {
		return nil, false, err
	}
	fillCategory := a.categoryID != nil
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
				return nil, false, err
			}
			id, err := vo.ParseId(p.ID)
			if err != nil {
				return nil, false, err
			}
			payeeID, newPayee = &id, &id
		}
	}
	if !fillCategory && payeeID == nil {
		return nil, false, nil
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
		return nil, false, err
	}
	if err := s.repo.UpdateLink(ctx, l); err != nil {
		return nil, false, err
	}
	if err := s.repo.InsertTransactionSuggestions(ctx, []model.ImportTransactionSuggestion{record}); err != nil {
		return nil, false, err
	}
	return newPayee, true, nil
}

// openAssignments gives each uncategorised row to be joined to an open
// suggestion before the AI call that suggestion's values, only for the fields
// the row lacks.
func openAssignments(joins map[vo.Id][]vo.Id, open []model.ImportSuggestion, rows []suggestRow) []assignment {
	byID := make(map[vo.Id]model.ImportSuggestion, len(open))
	for _, o := range open {
		byID[o.ID] = o
	}
	joinedTo := map[vo.Id]model.ImportSuggestion{}
	for id, links := range joins {
		o, ok := byID[id]
		if !ok {
			continue
		}
		for _, l := range links {
			joinedTo[l] = o
		}
	}
	var out []assignment
	for _, r := range rows {
		o, ok := joinedTo[r.Link.ID]
		if !ok || !r.Missing.Category {
			continue
		}
		a := assignment{row: r}
		a.categoryID = o.CategoryID
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
