package user

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// GetLanguage loads the user's last selected UI language (empty string if
// never set), used by the /mcp language fallback (see glue_language.go).
func (s *Service) GetLanguage(ctx context.Context, userID vo.Id) (string, error) {
	return s.repo.GetLanguage(ctx, userID)
}
