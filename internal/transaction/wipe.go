package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	wipeCorrectionPrefix   = "Original transaction deleted"
	wipeDescriptionMaxRune = 255
)

// RewriteForDeletedUser is wipe steps 5 and 6 for one user: what the user
// leaves in other people's accounts is rewritten so that every surviving
// account keeps its balance on every date. It runs on the caller's
// transaction context, before the user row and its cascade go.
func (s *Service) RewriteForDeletedUser(ctx context.Context, userID vo.Id) error {
	if err := s.repo.BumpAccountsLosingReferences(ctx, userID); err != nil {
		return err
	}
	if err := s.repo.ReassignAuthorInForeignAccounts(ctx, userID); err != nil {
		return err
	}
	transfers, err := s.repo.ListCrossOwnerTransfers(ctx, userID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	for _, t := range transfers {
		c, err := wipeCorrection(t, userID, now)
		if err != nil {
			return err
		}
		if err := s.repo.ReplaceTransferWithCorrection(ctx, t, c); err != nil {
			return err
		}
	}
	return nil
}

// wipeCorrection mirrors the surviving leg: the outgoing leg becomes an
// expense of amount at sort_key, the incoming one an income of
// amount_recipient at recipient_sort_key. A NULL amount_recipient never
// counted toward the recipient's balance, so that leg gets no correction.
func wipeCorrection(t CrossOwnerTransfer, userID vo.Id, now time.Time) (*WipeCorrection, error) {
	c := &WipeCorrection{
		ID:          vo.NewId(),
		Description: wipeDescription(t.Description),
		SpentAt:     t.SpentAt,
		CreatedAt:   now,
	}
	if t.RecipientOwnerID.Equal(userID) {
		c.AuthorID, c.AccountID = t.SourceOwnerID, t.SourceAccountID
		c.Type, c.Amount, c.SortKey = model.TransactionTypeExpense, t.Amount, t.SortKey
		return c, nil
	}
	if t.AmountRecipient == nil {
		return nil, nil
	}
	// An unplaced incoming leg cannot keep its position; refusing beats a
	// correction the list cannot order.
	if t.RecipientSortKey == nil || *t.RecipientSortKey == "" {
		return nil, fmt.Errorf("transfer %s has no recipient sort key", t.ID)
	}
	c.AuthorID, c.AccountID = t.RecipientOwnerID, t.RecipientAccountID
	c.Type, c.Amount, c.SortKey = model.TransactionTypeIncome, *t.AmountRecipient, *t.RecipientSortKey
	return c, nil
}

func wipeDescription(original string) string {
	d := wipeCorrectionPrefix
	if original != "" {
		d += ": " + original
	}
	if r := []rune(d); len(r) > wipeDescriptionMaxRune {
		d = string(r[:wipeDescriptionMaxRune])
	}
	return d
}
