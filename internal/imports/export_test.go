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
	return s.applyEvent(ctx, src, eventID, ev, runID, correctAmount, rules)
}
