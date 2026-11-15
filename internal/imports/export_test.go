package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ApplyEventForTest exposes applyEvent to the black-box test package
// (internal/imports/ingest_test.go is package imports_test).
func (s *Service) ApplyEventForTest(ctx context.Context, src *model.ImportSource, eventID vo.Id, ev model.IngestEvent, runID *vo.Id, correctAmount bool) (string, bool, error) {
	rules, err := s.loadRules(ctx, src)
	if err != nil {
		return "", false, err
	}
	status, amountUpdated, _, err := s.applyEvent(ctx, src, eventID, ev, runID, correctAmount, rules, nil)
	return status, amountUpdated, err
}

// LockSuggestOwnerForTest holds an owner's suggestion-job lock until the
// returned func runs.
func (s *Service) LockSuggestOwnerForTest(owner vo.Id) func() { return s.suggestLocks.lock(owner) }

const SuggestTimeoutForTest = suggestTimeout

var CandidateLeg = candidateLeg
