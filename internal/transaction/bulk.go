package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// maxBulkUpdateIds caps bulk_update_transactions: a batch of validated updates
// runs sequentially inside one transaction, and MCP is not a high-QPS path, but
// an unbounded id list would hold the tx open indefinitely.
const maxBulkUpdateIds = 100

// BulkUpdateTransactions re-classifies (sets or clears category/payee/tag on)
// an explicit list of transactions in one all-or-nothing call. This is
// MCP-only — there is no REST route.
//
// It deliberately does NOT run a raw UPDATE ... WHERE id IN: that would bypass
// the single-update invariants and let a caller e.g. attach a category to a
// transfer. Instead, for every id inside one s.tx.WithTx, it loads the
// transaction and applies only the requested classification change through
// the SAME validated path UpdateTransaction uses — checkWriteAccess (does the
// caller have write access to the transaction's account?), checkReferences
// (is a newly-set category/payee/tag owned by the caller?), and the
// transfer/non-transfer classification rule (a transfer may carry no
// category/payee/tag at all; a non-transfer must keep a category) — so a
// transfer rejecting a category, a foreign reference, an inaccessible
// account, or a missing id all surface as an error and roll back the whole
// batch; nothing is partially applied.
func (s *Service) BulkUpdateTransactions(ctx context.Context, userID vo.Id, req model.BulkUpdateTransactionsRequest) (*model.BulkUpdateTransactionsResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(req.Ids) > maxBulkUpdateIds {
		return nil, errs.NewValidation("at most 100 transactions per bulk update; batch the rest")
	}
	ids := make([]vo.Id, 0, len(req.Ids))
	for _, raw := range req.Ids {
		id, err := vo.ParseId(raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	var newCategoryID, newPayeeID, newTagID *vo.Id
	if req.CategoryId != nil {
		id, err := vo.ParseId(*req.CategoryId)
		if err != nil {
			return nil, err
		}
		newCategoryID = &id
	}
	if req.PayeeId != nil {
		id, err := vo.ParseId(*req.PayeeId)
		if err != nil {
			return nil, err
		}
		newPayeeID = &id
	}
	if req.TagId != nil {
		id, err := vo.ParseId(*req.TagId)
		if err != nil {
			return nil, err
		}
		newTagID = &id
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		for _, id := range ids {
			t, gerr := s.repo.GetByID(ctx, id)
			if gerr != nil {
				return gerr
			}
			if aerr := s.checkWriteAccess(ctx, userID, t.AccountID, "transaction.transaction.not_available"); aerr != nil {
				return aerr
			}
			if t.Type.IsTransfer() {
				if newCategoryID != nil || newPayeeID != nil || newTagID != nil || req.ClearCategory || req.ClearPayee || req.ClearTag {
					return errs.NewValidation("transaction " + id.String() + " is a transfer and cannot carry a category, payee, or tag")
				}
				continue
			}

			st := model.NewState{
				ID: t.ID, UserID: t.UserID, Type: t.Type, AccountID: t.AccountID,
				Amount: t.Amount, Description: t.Description, SpentAt: t.SpentAt,
				CreatedAt: t.CreatedAt, UpdatedAt: now,
				CategoryID: t.CategoryID, PayeeID: t.PayeeID, TagID: t.TagID,
			}
			switch {
			case req.ClearCategory:
				st.CategoryID = nil
			case newCategoryID != nil:
				st.CategoryID = newCategoryID
			}
			if st.CategoryID == nil {
				return errs.NewValidation("Validation failed",
					errs.FieldError{Key: "categoryId", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
			}
			switch {
			case req.ClearPayee:
				st.PayeeID = nil
			case newPayeeID != nil:
				st.PayeeID = newPayeeID
			}
			switch {
			case req.ClearTag:
				st.TagID = nil
			case newTagID != nil:
				st.TagID = newTagID
			}

			if rerr := s.checkReferences(ctx, userID, st); rerr != nil {
				return rerr
			}
			t.Update(st, now)
			if serr := s.repo.Save(ctx, t); serr != nil {
				return serr
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &model.BulkUpdateTransactionsResult{Updated: len(ids)}, nil
}
