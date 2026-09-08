package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const maxSyncRangeDays = 400

const accountFailedMessage = "Import failed for this account"

// Sync is the pull-side import: fetch the range from the provider, then run
// every account through the same pipeline a push event uses. The fetch runs
// outside any DB transaction (it may take seconds); each account is its own
// transaction so one bad account cannot roll back the others.
func (s *Service) Sync(ctx context.Context, userID vo.Id, req model.SyncImportSourceRequest) (*model.SyncImportSourceResult, error) {
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopeSync, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopeSync, userID.String())
	}
	src, p, err := s.pullSource(ctx, userID, req.SourceId)
	if err != nil {
		return nil, err
	}
	start, end, err := s.syncRange(ctx, req)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "source_id", src.ID.String())
	params, _ := json.Marshal(map[string]string{"startDate": req.StartDate, "endDate": end.Format(model.ImportDateLayout)})
	run := &model.ImportRun{
		ID: vo.NewId(), UserID: userID, SourceID: src.ID, Provider: src.Provider, Params: string(params),
		Status: model.ImportRunStatusRunning, Trigger: model.ImportRunTriggerManual, StartedAt: s.clk.Now().UTC(), Errors: []model.ImportRunError{},
	}
	if err := s.repo.InsertRun(ctx, run); err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "run_id", run.ID.String())
	// Once the run row exists every write that finalizes it must outlive the
	// request: a browser navigating away mid-fetch cancels ctx, and a run left
	// at "running" is shown forever with nothing able to clear it. The fetch
	// itself keeps the request ctx, so a disconnect still stops the bridge call.
	fin := context.WithoutCancel(ctx)
	fetched, ferr := p.FetchTransactions(ctx, Credential{AccessURL: req.AccessUrl}, FetchRequest{StartDate: start, EndDate: end.AddDate(0, 0, 1)})
	if ferr != nil {
		run.Status = model.ImportRunStatusFailed
		if err := s.finishRun(fin, run); err != nil {
			reqctx.AddLogAttr(ctx, "finish_run_error", true)
		}
		return nil, mapProviderErr(ferr)
	}
	for _, w := range fetched.Warnings {
		run.Errors = append(run.Errors, model.ImportRunError{Message: w})
	}
	byAccount := map[string][]model.ExternalTransaction{}
	for _, t := range fetched.Transactions {
		byAccount[t.ExternalAccountID] = append(byAccount[t.ExternalAccountID], t)
	}
	for _, a := range fetched.Accounts {
		if err := s.syncAccount(fin, src, run, a, byAccount[a.ID]); err != nil {
			run.Errors = append(run.Errors, model.ImportRunError{ExternalAccountId: a.ID, Message: accountFailedMessage})
			reqctx.AddLogAttr(ctx, "account_error", a.ID)
			// Type only, never the error text: it may originate from the
			// transaction feature and could carry a payee/account name.
			reqctx.AddLogAttr(ctx, "account_error_type", fmt.Sprintf("%T", err))
		}
	}
	switch {
	case len(fetched.Accounts) == 0 && len(run.Errors) > 0:
		run.Status = model.ImportRunStatusFailed
	case len(run.Errors) > 0 || run.FailedCount > 0:
		// Rows that failed to parse are a real problem the UI must not report
		// as a clean run, even when no account failed outright.
		run.Status = model.ImportRunStatusPartial
	default:
		run.Status = model.ImportRunStatusCompleted
	}
	if err := s.finishRun(fin, run); err != nil {
		return nil, err
	}
	if run.Status != model.ImportRunStatusFailed {
		// Re-read the source: the fetch may have run for seconds, and writing
		// back the row loaded before it would revert a concurrent reconnect
		// (create-source overwriting the ciphertext/name in the meantime).
		fresh, err := s.repo.GetSource(fin, src.ID)
		if err != nil {
			return nil, err
		}
		synced := s.clk.Now().UTC()
		fresh.LastSyncedAt, fresh.UpdatedAt = &synced, synced
		if err := s.repo.UpdateSource(fin, fresh); err != nil {
			return nil, err
		}
		src = fresh
	}
	accounts, err := s.externalAccountResults(ctx, src, fetched.Accounts)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "run_status", run.Status)
	return &model.SyncImportSourceResult{Run: runResult(run), Accounts: accounts}, nil
}

// syncAccount stores and applies one account's rows in a single transaction.
// A duplicate payload (same event already stored) is skipped silently, so a
// re-run of an overlapping range is a no-op for rows already seen.
func (s *Service) syncAccount(ctx context.Context, src *model.ImportSource, run *model.ImportRun, account model.ExternalAccount, rows []model.ExternalTransaction) error {
	snapshot := *run
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, row := range rows {
			payload := EncodeSimpleFINEvent(row)
			ev := &model.ImportEvent{
				ID: vo.NewId(), SourceID: src.ID, RunID: &run.ID, Payload: string(payload), PayloadHash: HashPayload(payload),
				Status: model.ImportEventStatusProcessed, ReceivedAt: s.clk.Now().UTC(),
			}
			inserted, err := s.repo.InsertEvent(ctx, ev)
			if err != nil {
				return err
			}
			if !inserted {
				continue
			}
			parsed, perr := s.parse(ctx, src, ev)
			if perr != nil {
				msg := perr.Error()
				if err := s.repo.UpdateEventStatus(ctx, ev.ID, model.ImportEventStatusFailed, &msg); err != nil {
					return err
				}
				run.FailedCount++
				continue
			}
			if parsed.Currency == "" {
				parsed.Currency = account.Currency
			}
			status, amountUpdated, err := s.applyEvent(ctx, src, ev.ID, parsed, &run.ID, true)
			if err != nil {
				return err
			}
			switch status {
			case model.ImportIngestStatusCreated:
				run.ImportedCount++
			case model.ImportIngestStatusMatched:
				run.MatchedCount++
				if amountUpdated {
					run.AmountsUpdatedCount++
				}
			case model.ImportIngestStatusQueued:
				run.QueuedCount++
			case model.ImportIngestStatusSkipped:
				run.SkippedCount++
			}
		}
		return nil
	})
	if err != nil {
		*run = snapshot // the transaction rolled back; so do its counters
	}
	return err
}

func (s *Service) finishRun(ctx context.Context, run *model.ImportRun) error {
	finished := s.clk.Now().UTC()
	run.FinishedAt = &finished
	return s.repo.UpdateRun(ctx, run)
}

// syncRange applies the range rules: end defaults to the caller's today,
// start must not follow end, and the span is capped so one click cannot
// pull a decade of history.
func (s *Service) syncRange(ctx context.Context, req model.SyncImportSourceRequest) (start, end time.Time, err error) {
	loc := reqctx.Location(ctx)
	start, _ = time.ParseInLocation(model.ImportDateLayout, req.StartDate, loc) // validated by the DTO
	if req.EndDate == "" {
		now := s.clk.Now().In(loc)
		end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	} else {
		end, _ = time.ParseInLocation(model.ImportDateLayout, req.EndDate, loc)
	}
	if start.After(end) || end.Sub(start) > maxSyncRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, &errs.ValidationError{Msg: "Invalid date range", MsgCode: errs.CodeImportSyncRangeInvalid}
	}
	return start, end, nil
}
