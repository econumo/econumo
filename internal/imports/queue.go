package imports

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) GetQueue(ctx context.Context, userID vo.Id) (*model.GetImportQueueResult, error) {
	out := &model.GetImportQueueResult{
		Queued: []model.ImportQueuedEventResult{}, Skipped: []model.ImportQueuedEventResult{}, Failed: []model.ImportFailedEventResult{},
	}
	sources, err := s.repo.ListSourcesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		src := &sources[i]
		accountLinks, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return nil, err
		}
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return nil, err
		}
		for j := range ledger {
			l := &ledger[j]
			if l.Status != model.ImportLinkStatusQueued && l.Status != model.ImportLinkStatusSkipped {
				continue
			}
			entry, err := s.queuedEntry(ctx, src, accountLinks, l)
			if err != nil {
				return nil, err
			}
			if l.Status == model.ImportLinkStatusQueued {
				out.Queued = append(out.Queued, entry)
			} else {
				out.Skipped = append(out.Skipped, entry)
			}
		}
		failed, err := s.repo.ListEventsBySourceStatus(ctx, src.ID, model.ImportEventStatusFailed)
		if err != nil {
			return nil, err
		}
		for _, ev := range failed {
			out.Failed = append(out.Failed, model.ImportFailedEventResult{
				EventId: ev.ID.String(), SourceId: src.ID.String(), ReceivedAt: ev.ReceivedAt.Format(datetime.Layout),
				Error: derefString(ev.ParseError), Payload: ev.Payload,
			})
		}
	}
	return out, nil
}

func (s *Service) queuedEntry(ctx context.Context, src *model.ImportSource, accountLinks []model.ImportAccountLink, l *model.ImportTransactionLink) (model.ImportQueuedEventResult, error) {
	entry := model.ImportQueuedEventResult{
		LinkId: l.ID.String(), SourceId: src.ID.String(), ExternalAccountId: l.ExternalAccountID,
		Payee: l.ExternalPayee, Amount: vo.NewDecimal(l.ExternalAmount).String(), Currency: derefString(l.ExternalCurrency),
		Type: model.TransactionTypeExpense.Alias(), PostedAt: l.ExternalPostedAt.Format(datetime.Layout), Reason: model.ImportQueueReasonUnmapped,
	}
	if ev, ok, err := s.reparse(ctx, src, l); err != nil {
		return entry, err
	} else if ok {
		entry.Type = ev.Type.Alias()
	}
	al := findAccountLink(accountLinks, l.ExternalAccountID)
	if al == nil || al.Mode != model.ImportAccountLinkModeImport || al.AccountID == nil {
		return entry, nil
	}
	entry.AccountId = al.AccountID.String()
	deleted, err := s.accounts.AccountDeleted(ctx, *al.AccountID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			entry.Reason = model.ImportQueueReasonAccountDeleted
			return entry, nil
		}
		return entry, err
	}
	if deleted {
		entry.Reason = model.ImportQueueReasonAccountDeleted
		return entry, nil
	}
	entry.Reason = model.ImportQueueReasonNoRate
	return entry, nil
}

// ImportQueuedEvent is the manual path out of the queue: the user reviewed
// the tap and submits the transaction they want (possibly edited). The link
// records what they chose so a later rule engine can learn from it.
func (s *Service) ImportQueuedEvent(ctx context.Context, userID vo.Id, req model.ImportQueuedEventRequest) (*model.CreateTransactionResult, error) {
	var out *model.CreateTransactionResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		link, _, err := s.ownedLink(ctx, userID, req.LinkId)
		if err != nil {
			return err
		}
		if link.Status != model.ImportLinkStatusQueued {
			return &errs.ValidationError{Msg: "This event is not queued", MsgCode: errs.CodeImportLinkNotQueued}
		}
		res, err := s.txns.CreateTransaction(ctx, userID, req.Transaction)
		if err != nil {
			return err
		}
		txID, err := vo.ParseId(res.Item.Id)
		if err != nil {
			return err
		}
		link.Status = model.ImportLinkStatusLinked
		link.TransactionID = &txID
		link.AppliedCategoryID = parseOptionalIDField(req.Transaction.CategoryId)
		link.AppliedPayeeID = parseOptionalIDField(req.Transaction.PayeeId)
		link.AppliedTagID = parseOptionalIDField(req.Transaction.TagId)
		if err := s.repo.UpdateLink(ctx, link); err != nil {
			return err
		}
		// CreateTransaction built res before the link existed, so it reports
		// isImported: 0 for a transaction that is, as of this line, imported.
		res.Item.IsImported = 1
		out = res
		return nil
	})
	return out, err
}

func parseOptionalIDField(raw *string) *vo.Id {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	id, err := vo.ParseId(strings.TrimSpace(*raw))
	if err != nil {
		return nil
	}
	return &id
}

func (s *Service) SkipQueuedEvent(ctx context.Context, userID vo.Id, req model.ImportLinkActionRequest) (*model.GetImportQueueResult, error) {
	return s.flipLink(ctx, userID, req.LinkId, model.ImportLinkStatusQueued, model.ImportLinkStatusSkipped, errs.CodeImportLinkNotQueued, "This event is not queued")
}

func (s *Service) UnskipQueuedEvent(ctx context.Context, userID vo.Id, req model.ImportLinkActionRequest) (*model.GetImportQueueResult, error) {
	return s.flipLink(ctx, userID, req.LinkId, model.ImportLinkStatusSkipped, model.ImportLinkStatusQueued, errs.CodeImportLinkNotSkipped, "This event is not skipped")
}

func (s *Service) flipLink(ctx context.Context, userID vo.Id, rawID, from, to, code, msg string) (*model.GetImportQueueResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		link, _, err := s.ownedLink(ctx, userID, rawID)
		if err != nil {
			return err
		}
		if link.Status != from {
			return &errs.ValidationError{Msg: msg, MsgCode: code}
		}
		link.Status = to
		return s.repo.UpdateLink(ctx, link)
	})
	if err != nil {
		return nil, err
	}
	return s.GetQueue(ctx, userID)
}

// DiscardEvent drops a failed inbox event. Processed events are refused:
// their hash is what makes a re-fired push a duplicate.
func (s *Service) DiscardEvent(ctx context.Context, userID vo.Id, req model.DiscardImportEventRequest) (*model.GetImportQueueResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		ev, _, err := s.ownedEvent(ctx, userID, req.EventId)
		if err != nil {
			return err
		}
		if ev.Status != model.ImportEventStatusFailed {
			return &errs.ValidationError{Msg: "This event did not fail", MsgCode: errs.CodeImportEventNotFailed}
		}
		return s.repo.DeleteEvent(ctx, ev.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetQueue(ctx, userID)
}

// GetTransactionImportList is the provenance view of one transaction: every
// ledger row pointing at it whose source the caller owns.
func (s *Service) GetTransactionImportList(ctx context.Context, userID vo.Id, req model.TransactionImportListRequest) (*model.GetTransactionImportListResult, error) {
	out := &model.GetTransactionImportListResult{Items: []model.TransactionImportLinkResult{}}
	txID, err := vo.ParseId(strings.TrimSpace(req.TransactionId))
	if err != nil {
		return out, nil
	}
	links, err := s.repo.ListLinksByTransaction(ctx, txID)
	if err != nil {
		return nil, err
	}
	sources := map[vo.Id]*model.ImportSource{}
	for _, l := range links {
		src, seen := sources[l.SourceID]
		if !seen {
			src, err = s.repo.GetSource(ctx, l.SourceID)
			if err != nil {
				if _, ok := errs.AsNotFound(err); ok {
					continue
				}
				return nil, err
			}
			sources[l.SourceID] = src
		}
		if src.UserID != userID {
			continue
		}
		out.Items = append(out.Items, model.TransactionImportLinkResult{
			Id: l.ID.String(), SourceId: src.ID.String(), Provider: src.Provider, SourceName: src.Name,
			ExternalAccountId: l.ExternalAccountID, ExternalTransactionId: l.ExternalTransactionID,
			ExternalPayee: l.ExternalPayee, ExternalAmount: vo.NewDecimal(l.ExternalAmount).String(), ExternalCurrency: derefString(l.ExternalCurrency),
			ExternalPostedAt: l.ExternalPostedAt.Format(datetime.Layout), Status: l.Status, ImportedAt: l.ImportedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}
