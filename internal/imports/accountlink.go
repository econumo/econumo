package imports

import (
	"context"
	"strconv"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// LinkAccount maps a card to an account and converts the card's queued taps.
func (s *Service) LinkAccount(ctx context.Context, userID vo.Id, req model.LinkImportAccountRequest) (*model.UpdateImportAccountResult, error) {
	var out *model.UpdateImportAccountResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.SourceId)
		if err != nil {
			return err
		}
		accountID, err := vo.ParseId(strings.TrimSpace(req.AccountId))
		if err != nil {
			return errs.NewNotFound("Account not found")
		}
		owner, err := s.accounts.AccountOwner(ctx, accountID)
		if err != nil {
			return err
		}
		if owner != userID {
			return errs.NewNotFound("Account not found")
		}
		deleted, err := s.accounts.AccountDeleted(ctx, accountID)
		if err != nil {
			return err
		}
		if deleted {
			return &errs.ValidationError{Msg: "Account is deleted", MsgCode: errs.CodeTransactionAccountDeleted}
		}
		ext := normalizeExternalAccountID(req.ExternalAccountId)
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		accountCode, err := s.accounts.AccountCurrencyCode(ctx, accountID)
		if err != nil {
			return err
		}
		// A card that only ever reports one currency is IN that currency; mapping it
		// onto an account in another one is a misconfiguration, not a conversion job.
		cardCode := uniformCurrency(ledger, ext)
		if cardCode != "" && cardCode != accountCode {
			return &errs.ValidationError{Msg: "Card currency does not match the account", MsgCode: errs.CodeImportCurrencyMismatch}
		}
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if existing := findAccountLink(links, ext); existing != nil {
			if existing.Mode == model.ImportAccountLinkModeImport {
				return &errs.ValidationError{Msg: "This card is already linked", MsgCode: errs.CodeImportAccountLinkExists}
			}
			existing.Mode = model.ImportAccountLinkModeImport // map-instead over an ignored card
			existing.AccountID = &accountID
			existing.ExternalCurrency = optionalString(cardCode)
			existing.UpdatedAt = now
			if err := s.repo.UpdateAccountLink(ctx, existing); err != nil {
				return err
			}
		} else {
			if err := s.repo.InsertAccountLink(ctx, &model.ImportAccountLink{
				ID: vo.NewId(), SourceID: src.ID, ExternalAccountID: ext, ExternalName: ext,
				ExternalCurrency: optionalString(cardCode), AccountID: &accountID,
				Mode: model.ImportAccountLinkModeImport, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		run, err := s.convertQueued(ctx, src, ext, ledger)
		if err != nil {
			return err
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.UpdateImportAccountResult{Item: *item, Run: run}
		return nil
	})
	return out, err
}

// uniformCurrency is the card's currency when every ledger row agrees; ""
// for a card with no rows or mixed rows.
func uniformCurrency(ledger []model.ImportTransactionLink, ext string) string {
	code := ""
	for _, l := range ledger {
		if !strings.EqualFold(l.ExternalAccountID, ext) || l.ExternalCurrency == nil {
			continue
		}
		if code == "" {
			code = *l.ExternalCurrency
		} else if code != *l.ExternalCurrency {
			return ""
		}
	}
	return code
}

// convertQueued re-runs stages 2-3 for every queued tap on the card, now
// that it has a destination. Rows that still cannot be placed (no rate)
// stay queued and count as skipped; the run is "partial" then.
func (s *Service) convertQueued(ctx context.Context, src *model.ImportSource, ext string, ledger []model.ImportTransactionLink) (*model.ImportRunResult, error) {
	now := s.clk.Now().UTC()
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: src.UserID, SourceID: src.ID, Provider: src.Provider,
		Params: `{"externalAccountId":` + strconv.Quote(ext) + `}`, Status: model.ImportRunStatusRunning, StartedAt: now,
	}
	if err := s.repo.InsertRun(ctx, run); err != nil {
		return nil, err
	}
	for i := range ledger {
		l := &ledger[i]
		if l.Status != model.ImportLinkStatusQueued || !strings.EqualFold(l.ExternalAccountID, ext) {
			continue
		}
		ev, ok, err := s.reparse(ctx, src, l)
		if err != nil {
			return nil, err
		}
		if !ok {
			run.SkippedCount++
			continue
		}
		r, err := s.resolve(ctx, src, ev)
		if err != nil {
			return nil, err
		}
		if r.status != "" {
			run.SkippedCount++
			continue
		}
		txID, adopted, err := s.place(ctx, src, ev, r)
		if err != nil {
			return nil, err
		}
		l.Status = model.ImportLinkStatusLinked
		l.TransactionID = &txID
		l.RunID = &run.ID
		if err := s.repo.UpdateLink(ctx, l); err != nil {
			return nil, err
		}
		if adopted {
			run.MatchedCount++
		} else {
			run.ImportedCount++
		}
	}
	run.Status = model.ImportRunStatusCompleted
	if run.SkippedCount > 0 {
		run.Status = model.ImportRunStatusPartial
	}
	finished := s.clk.Now().UTC()
	run.FinishedAt = &finished
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	return &model.ImportRunResult{
		Id: run.ID.String(), Status: run.Status, ImportedCount: run.ImportedCount,
		MatchedCount: run.MatchedCount, SkippedCount: run.SkippedCount, FailedCount: run.FailedCount,
	}, nil
}

// reparse rebuilds the IngestEvent behind a ledger row from its stored
// inbox event. ok=false when the event is gone or no longer parses.
func (s *Service) reparse(ctx context.Context, src *model.ImportSource, l *model.ImportTransactionLink) (model.IngestEvent, bool, error) {
	if l.EventID == nil {
		return model.IngestEvent{}, false, nil
	}
	stored, err := s.repo.GetEvent(ctx, *l.EventID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return model.IngestEvent{}, false, nil
		}
		return model.IngestEvent{}, false, err
	}
	ev, perr := s.parse(src, stored)
	if perr != nil {
		return model.IngestEvent{}, false, nil
	}
	return ev, true, nil
}

func (s *Service) IgnoreAccount(ctx context.Context, userID vo.Id, req model.ImportAccountActionRequest) (*model.UpdateImportAccountResult, error) {
	var out *model.UpdateImportAccountResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.SourceId)
		if err != nil {
			return err
		}
		ext := normalizeExternalAccountID(req.ExternalAccountId)
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if existing := findAccountLink(links, ext); existing != nil {
			existing.Mode = model.ImportAccountLinkModeIgnore
			existing.AccountID = nil
			existing.UpdatedAt = now
			if err := s.repo.UpdateAccountLink(ctx, existing); err != nil {
				return err
			}
		} else if err := s.repo.InsertAccountLink(ctx, &model.ImportAccountLink{
			ID: vo.NewId(), SourceID: src.ID, ExternalAccountID: ext, ExternalName: ext,
			Mode: model.ImportAccountLinkModeIgnore, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		for i := range ledger {
			if ledger[i].Status == model.ImportLinkStatusQueued && strings.EqualFold(ledger[i].ExternalAccountID, ext) {
				ledger[i].Status = model.ImportLinkStatusSkipped
				if err := s.repo.UpdateLink(ctx, &ledger[i]); err != nil {
					return err
				}
			}
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.UpdateImportAccountResult{Item: *item}
		return nil
	})
	return out, err
}

// UnlinkAccount forgets a card: its mapping, its queued taps and their inbox
// events. Seen rows (linked/skipped) stay — they are the dedupe memory.
func (s *Service) UnlinkAccount(ctx context.Context, userID vo.Id, req model.ImportAccountActionRequest) (*model.UpdateImportAccountResult, error) {
	var out *model.UpdateImportAccountResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.SourceId)
		if err != nil {
			return err
		}
		ext := normalizeExternalAccountID(req.ExternalAccountId)
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		if existing := findAccountLink(links, ext); existing != nil {
			if err := s.repo.DeleteAccountLink(ctx, existing.ID); err != nil {
				return err
			}
		}
		ledger, err := s.repo.ListLinksBySource(ctx, src.ID)
		if err != nil {
			return err
		}
		var eventIDs []vo.Id
		for _, l := range ledger {
			if l.Status == model.ImportLinkStatusQueued && strings.EqualFold(l.ExternalAccountID, ext) && l.EventID != nil {
				eventIDs = append(eventIDs, *l.EventID)
			}
		}
		// the purge folds case in SQL, so one call covers every spelling this
		// card's taps arrived in
		if err := s.repo.DeleteQueuedLinksByExternalAccount(ctx, src.ID, ext); err != nil {
			return err
		}
		for _, id := range eventIDs {
			if err := s.repo.DeleteEvent(ctx, id); err != nil {
				return err
			}
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.UpdateImportAccountResult{Item: *item}
		return nil
	})
	return out, err
}
