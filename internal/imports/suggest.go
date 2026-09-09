package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SuggestRules is a stub: no AI provider is wired yet. Task 11 replaces this
// with the real implementation.
func (s *Service) SuggestRules(ctx context.Context, userID vo.Id, req model.SuggestImportRulesRequest) (*model.SuggestImportRulesResult, error) {
	return nil, &errs.ValidationError{Msg: "AI suggestions are not enabled on this server", MsgCode: errs.CodeImportAiDisabled}
}
