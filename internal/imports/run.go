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

// SourceHistoryPage is how many rows one history page carries.
const SourceHistoryPage = 50

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
	for i := range links {
		out.Links = append(out.Links, runLinkResult(&links[i]))
	}
	return out, nil
}

// GetSourceHistory lists one source's rows — every tap a push provider
// delivered, whatever became of it — newest first, a page at a time. Rows a
// payload could not be parsed into never exist; those stay in the queue's
// failed list.
func (s *Service) GetSourceHistory(ctx context.Context, userID vo.Id, sourceID string, offset int) (*model.GetImportSourceHistoryResult, error) {
	src, err := s.ownedSource(ctx, userID, sourceID)
	if err != nil {
		return nil, err
	}
	// one row past the page tells whether another page follows
	links, err := s.repo.ListLinksBySourcePage(ctx, src.ID, SourceHistoryPage+1, max(offset, 0))
	if err != nil {
		return nil, err
	}
	out := &model.GetImportSourceHistoryResult{Items: make([]model.ImportRunLinkResult, 0, min(len(links), SourceHistoryPage))}
	for i := range links {
		if i == SourceHistoryPage {
			out.HasMore = true
			break
		}
		out.Items = append(out.Items, runLinkResult(&links[i]))
	}
	return out, nil
}

func runLinkResult(l *model.ImportTransactionLink) model.ImportRunLinkResult {
	return model.ImportRunLinkResult{
		Id: l.ID.String(), ExternalAccountId: l.ExternalAccountID, ExternalTransactionId: l.ExternalTransactionID,
		TransactionId: idString(l.TransactionID), Status: l.Status, ExternalPayee: l.ExternalPayee,
		ExternalAmount: l.ExternalAmount, ExternalCurrency: derefString(l.ExternalCurrency),
		ExternalPostedAt: l.ExternalPostedAt.Format(datetime.Layout),
	}
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
