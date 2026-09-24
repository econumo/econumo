package imports

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// IngestAppleWallet is the push endpoint's use case. The raw payload is
// persisted before anything is interpreted, so a broken body is a 'failed'
// event the user can see and discard — never a lost tap. Everything runs in
// one transaction: a failed create leaves no event, no ledger row.
func (s *Service) IngestAppleWallet(ctx context.Context, userID vo.Id, payload []byte) (*model.IngestEventResult, error) {
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeIngest, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeIngest, userID.String())
	}
	var res *model.IngestEventResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.repo.GetSourceByUserProvider(ctx, userID, model.ImportProviderAppleWallet)
		if err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return &errs.ValidationError{Msg: "Apple Wallet is not connected", MsgCode: errs.CodeImportSourceNotFound}
			}
			return err
		}
		ev := &model.ImportEvent{
			ID: vo.NewId(), SourceID: src.ID, Payload: string(payload), PayloadHash: HashPayload(payload),
			Status: model.ImportEventStatusProcessed, ReceivedAt: s.clk.Now().UTC(),
		}
		inserted, err := s.repo.InsertEvent(ctx, ev)
		if err != nil {
			return err
		}
		if !inserted {
			res = &model.IngestEventResult{Status: model.ImportIngestStatusDuplicate}
			return nil
		}
		status, err := s.processEvent(ctx, src, ev)
		if err != nil {
			return err
		}
		res = &model.IngestEventResult{Status: status, EventId: ev.ID.String()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// RetryEvent re-runs a failed event's parse + pipeline. The payload is what
// it was; a retry is useful after the user mapped the card or the server was
// upgraded with a more tolerant parser.
func (s *Service) RetryEvent(ctx context.Context, userID vo.Id, req model.RetryImportEventRequest) (*model.IngestEventResult, error) {
	var res *model.IngestEventResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		ev, src, err := s.ownedEvent(ctx, userID, req.EventId)
		if err != nil {
			return err
		}
		if ev.Status != model.ImportEventStatusFailed {
			return &errs.ValidationError{Msg: "This event did not fail", MsgCode: errs.CodeImportEventNotFailed}
		}
		status, err := s.processEvent(ctx, src, ev)
		if err != nil {
			return err
		}
		res = &model.IngestEventResult{Status: status, EventId: ev.ID.String()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// processEvent runs stages 0-3 for one stored event and records the outcome
// on the event row. It returns the ingest status; only infrastructure
// failures are errors.
func (s *Service) processEvent(ctx context.Context, src *model.ImportSource, ev *model.ImportEvent) (string, error) {
	parsed, perr := s.parse(ctx, src, ev)
	if perr != nil {
		msg := perr.Error()
		return model.ImportIngestStatusFailed, s.repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusFailed, &msg)
	}
	rules, err := s.loadRules(ctx, src)
	if err != nil {
		return "", err
	}
	// A retried event is reprocessed exactly like the original push: it never
	// carries a run id and never corrects an already-adopted amount.
	status, _, err := s.applyEvent(ctx, src, ev.ID, parsed, nil, false, rules)
	if err != nil {
		return "", err
	}
	return status, s.repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusProcessed, nil)
}

func (s *Service) parse(ctx context.Context, src *model.ImportSource, ev *model.ImportEvent) (model.IngestEvent, error) {
	p, ok := s.parsers[src.Provider]
	if !ok {
		return model.IngestEvent{}, errors.New("unsupported provider " + src.Provider)
	}
	return p.ParseEvent(ctx, ev)
}

// resolution is stage 1+2's verdict for an event: where it goes and in what
// amount, or why it cannot go anywhere yet.
type resolution struct {
	accountID  vo.Id
	amount     string
	status     string // "" = import; ImportIngestStatusQueued / ImportIngestStatusSkipped otherwise
	skipRuleID *vo.Id // set with status=Skipped when a skip rule fired (nil = ignored card)
}

func (s *Service) resolve(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, rules ruleSet) (resolution, error) {
	links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
	if err != nil {
		return resolution{}, err
	}
	al := findAccountLink(links, ev.ExternalAccountID)
	if al == nil {
		return resolution{status: model.ImportIngestStatusQueued}, nil
	}
	if al.Mode == model.ImportAccountLinkModeIgnore || al.AccountID == nil {
		return resolution{status: model.ImportIngestStatusSkipped}, nil
	}
	deleted, err := s.accounts.AccountDeleted(ctx, *al.AccountID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return resolution{status: model.ImportIngestStatusQueued}, nil
		}
		return resolution{}, err
	}
	if deleted { // dormant link: keep the tap for when the user re-maps the card
		return resolution{status: model.ImportIngestStatusQueued}, nil
	}
	code, err := s.accounts.AccountCurrencyCode(ctx, *al.AccountID)
	if err != nil {
		return resolution{}, err
	}
	amount := ev.Amount
	if ev.Currency != "" && ev.Currency != code {
		converted, ok, err := s.converter.Convert(ctx, src.UserID, ev.Currency, code, ev.Amount, wallClock(ev.PostedAt))
		if err != nil {
			return resolution{}, err
		}
		if !ok {
			return resolution{status: model.ImportIngestStatusQueued}, nil
		}
		amount = converted
	}
	// A skip rule acts only on an event that could otherwise import: the
	// card is mapped and the amount converted. Unmapped/no-rate events queue
	// as before so the user still sees them.
	if id := rules.skip(ev); id != nil {
		return resolution{status: model.ImportIngestStatusSkipped, skipRuleID: id}, nil
	}
	return resolution{accountID: *al.AccountID, amount: amount}, nil
}

// applyEvent is stages 1-3 for a parsed event: the ledger check, the account
// resolution, and matching/creation. It always leaves exactly one ledger row
// for a new external key. runID is nil for a push event and the enclosing
// sync run's id for a pull event; correctAmount lets a tip-adopt overwrite a
// stale tap-time amount (a bank sync only — a push never rewrites a
// hand-entered amount).
func (s *Service) applyEvent(ctx context.Context, src *model.ImportSource, eventID vo.Id, ev model.IngestEvent, runID *vo.Id, correctAmount bool, rules ruleSet) (status string, amountUpdated bool, err error) {
	// The ledger stores the card name in its original case (the queue page
	// displays it), so the dedup lookup itself is case-insensitive on the
	// card name at the query layer (GetLinkByExternalKey) rather than being
	// canonicalized here.
	existing, err := s.repo.GetLinkByExternalKey(ctx, src.ID, ev.ExternalAccountID, ev.ExternalTransactionID)
	if err == nil {
		if existing.IsSeen() {
			return model.ImportIngestStatusDuplicate, false, nil
		}
		return model.ImportIngestStatusQueued, false, nil // still waiting on the user; nothing new to record
	} else if _, ok := errs.AsNotFound(err); !ok {
		return "", false, err
	}
	if ev.Currency == "" {
		// SimpleFIN rows carry no currency of their own — the account does.
		// A sync pre-fills this from the fetched account; this is the retry
		// path's fallback, reading the currency the account link recorded the
		// first time the account was seen.
		links, err := s.repo.ListAccountLinksBySource(ctx, src.ID)
		if err != nil {
			return "", false, err
		}
		if al := findAccountLink(links, ev.ExternalAccountID); al != nil && al.ExternalCurrency != nil {
			ev.Currency = *al.ExternalCurrency
		}
	}
	link := &model.ImportTransactionLink{
		ID: vo.NewId(), SourceID: src.ID, RunID: runID, EventID: &eventID,
		ExternalAccountID: ev.ExternalAccountID, ExternalTransactionID: ev.ExternalTransactionID,
		Status: model.ImportLinkStatusQueued, ExternalPayee: ev.Payee, ExternalDescription: ev.Description,
		ExternalAmount: ev.Amount, ExternalCurrency: optionalString(ev.Currency),
		ExternalPostedAt: wallClock(ev.PostedAt), ImportedAt: s.clk.Now().UTC(),
	}
	r, err := s.resolve(ctx, src, ev, rules)
	if err != nil {
		return "", false, err
	}
	switch r.status {
	case model.ImportIngestStatusQueued:
		return r.status, false, s.repo.InsertLink(ctx, link)
	case model.ImportIngestStatusSkipped:
		link.Status = model.ImportLinkStatusSkipped
		link.AppliedRuleID = r.skipRuleID
		return r.status, false, s.repo.InsertLink(ctx, link)
	}
	txID, adopted, amountUpdated, applied, err := s.place(ctx, src, ev, r, correctAmount, rules)
	if err != nil {
		return "", false, err
	}
	link.Status = model.ImportLinkStatusLinked
	link.TransactionID = &txID
	setApplied(link, applied)
	// the applied-label rows carry an FK on link_id, so the link goes in first
	if err := s.repo.InsertLink(ctx, link); err != nil {
		return "", false, err
	}
	if err := s.repo.ReplaceLinkAppliedLabels(ctx, link.ID, applied.LabelIDs); err != nil {
		return "", false, err
	}
	status = model.ImportIngestStatusCreated
	if adopted {
		status = model.ImportIngestStatusMatched
	}
	return status, amountUpdated, nil
}

// place is stage 3: adopt an existing transaction the matcher recognizes
// (adopted=true), else create one through the transaction feature's own use
// case. correctAmount lets a tip-adopt overwrite the candidate's tap-time
// amount with the posted one (amountUpdated=true); push events pass false
// (see applyEvent). applied is the classification the transaction ends up
// carrying: what the classify rules wrote on a create, what the adopted
// transaction already had on an adopt.
func (s *Service) place(ctx context.Context, src *model.ImportSource, ev model.IngestEvent, r resolution, correctAmount bool, rules ruleSet) (txID vo.Id, adopted bool, amountUpdated bool, applied model.ImportClassification, err error) {
	at := wallClock(ev.PostedAt)
	window := s.cfg.MatchDays
	if s.cfg.TipDays > window {
		window = s.cfg.TipDays
	}
	txs, err := s.lister.ListByAccount(ctx, r.accountID, at.AddDate(0, 0, -window), at.AddDate(0, 0, window+1))
	if err != nil {
		return vo.Id{}, false, false, model.ImportClassification{}, err
	}
	providers := map[vo.Id]string{}
	byID := make(map[vo.Id]*model.Transaction, len(txs))
	candidates := make([]model.ImportCandidate, 0, len(txs))
	for _, t := range txs {
		byID[t.ID] = t
		c := model.ImportCandidate{TransactionID: t.ID, Type: t.Type, Amount: t.Amount, SpentAt: t.SpentAt}
		links, err := s.repo.ListLinksByTransaction(ctx, t.ID)
		if err != nil {
			return vo.Id{}, false, false, model.ImportClassification{}, err
		}
		for _, l := range links {
			provider, seen := providers[l.SourceID]
			if !seen {
				ls, err := s.repo.GetSource(ctx, l.SourceID)
				if err != nil {
					return vo.Id{}, false, false, model.ImportClassification{}, err
				}
				provider = ls.Provider
				providers[l.SourceID] = provider
			}
			c.Links = append(c.Links, model.ImportCandidateLink{SourceID: l.SourceID, Provider: provider, ExternalAmount: l.ExternalAmount, ExternalPayee: l.ExternalPayee})
		}
		candidates = append(candidates, c)
	}
	matchEv := ev
	matchEv.Amount = r.amount // compare in the account's currency
	matchEv.PostedAt = at
	if m := Match(matchEv, src.ID, candidates, s.cfg); m.Kind != MatchCreate {
		if correctAmount && m.Kind == MatchTipAdopt && m.CorrectAmount {
			cand := byID[m.TransactionID]
			req := model.UpdateTransactionRequest{
				Id: cand.ID.String(), Type: cand.Type.Alias(), AccountId: cand.AccountID.String(),
				Amount: vo.NewFlexString(r.amount), Date: cand.SpentAt.Format(datetime.Layout),
				Description: optionalString(cand.Description),
				CategoryId:  idString2(cand.CategoryID), PayeeId: idString2(cand.PayeeID), TagId: idString2(cand.TagID),
				AccountRecipientId: idString2(cand.AccountRecipID),
			}
			if cand.AmountRecipient != nil {
				ar := vo.NewFlexString(*cand.AmountRecipient)
				req.AmountRecipient = &ar
			}
			if _, err := s.txns.UpdateTransaction(ctx, src.UserID, req); err != nil {
				return vo.Id{}, false, false, model.ImportClassification{}, err
			}
			applied, err := s.snapshotOf(ctx, m.TransactionID)
			if err != nil {
				return vo.Id{}, false, false, model.ImportClassification{}, err
			}
			return m.TransactionID, true, true, applied, nil
		}
		// An adopted transaction keeps whatever it already carries; rules
		// never rewrite a classification the user made by hand.
		applied, err := s.snapshotOf(ctx, m.TransactionID)
		if err != nil {
			return vo.Id{}, false, false, model.ImportClassification{}, err
		}
		return m.TransactionID, true, false, applied, nil
	}
	c := rules.classify(ev)
	res, err := s.txns.CreateTransaction(ctx, src.UserID, model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: ev.Type.Alias(), Amount: vo.NewFlexString(r.amount), AccountId: r.accountID.String(),
		Date: at.Format(datetime.Layout), Description: optionalString(ev.Payee),
		CategoryId: idString2(c.CategoryID), PayeeId: idString2(c.PayeeID), TagId: idString2(c.TagID), LabelIds: idStrings(c.LabelIDs),
	})
	if err != nil {
		return vo.Id{}, false, false, model.ImportClassification{}, err
	}
	txID, err = vo.ParseId(res.Item.Id)
	return txID, false, false, c, err
}
