package imports

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const RunListLimit = 50

func (s *Service) GetRunList(ctx context.Context, userID vo.Id, sourceID string) (*model.GetImportRunListResult, error) {
	var filter *vo.Id
	if sourceID != "" {
		src, err := s.ownedSource(ctx, userID, sourceID)
		if err != nil {
			return nil, err
		}
		filter = &src.ID
	}
	runs, err := s.repo.ListRunsByUser(ctx, userID, filter, RunListLimit)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportRunListResult{Items: make([]model.ImportRunResult, 0, len(runs))}
	for i := range runs {
		out.Items = append(out.Items, runResult(&runs[i]))
	}
	return out, nil
}

func (s *Service) GetRun(ctx context.Context, userID vo.Id, rawID string) (*model.GetImportRunResult, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, errs.NewNotFound("Import run not found")
	}
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if run.UserID != userID {
		return nil, errs.NewNotFound("Import run not found")
	}
	links, err := s.repo.ListLinksByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportRunResult{Item: runResult(run), Links: make([]model.ImportRunLinkResult, 0, len(links))}
	for _, l := range links {
		out.Links = append(out.Links, model.ImportRunLinkResult{
			Id: l.ID.String(), ExternalAccountId: l.ExternalAccountID, ExternalTransactionId: l.ExternalTransactionID,
			TransactionId: idString(l.TransactionID), Status: l.Status, ExternalPayee: l.ExternalPayee,
			ExternalAmount: l.ExternalAmount, ExternalCurrency: derefString(l.ExternalCurrency),
			ExternalPostedAt: l.ExternalPostedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

func runResult(r *model.ImportRun) model.ImportRunResult {
	out := model.ImportRunResult{
		Id: r.ID.String(), SourceId: r.SourceID.String(), Provider: r.Provider, Status: r.Status, Trigger: r.Trigger,
		ImportedCount: r.ImportedCount, MatchedCount: r.MatchedCount, AmountsUpdatedCount: r.AmountsUpdatedCount,
		QueuedCount: r.QueuedCount, SkippedCount: r.SkippedCount, FailedCount: r.FailedCount,
		Errors: r.Errors, StartedAt: r.StartedAt.Format(datetime.Layout),
	}
	if out.Errors == nil {
		out.Errors = []model.ImportRunError{}
	}
	if r.FinishedAt != nil {
		out.FinishedAt = r.FinishedAt.Format(datetime.Layout)
	}
	return out
}
