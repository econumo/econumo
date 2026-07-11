// Session lifecycle: creation at login and the opportunistic purge of dead
// rows.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// createSession mints a session row for a fresh login and returns the raw
// bearer token (the only moment it exists server-side).
func (s *Service) createSession(ctx context.Context, userID vo.Id, userAgent string, now time.Time) (string, error) {
	raw, hash, err := generateAccessToken(model.TokenKindSession)
	if err != nil {
		return "", err
	}
	exp := now.Add(SessionTTL)
	t := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: model.TokenKindSession, TokenHash: hash,
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
	}
	if userAgent != "" {
		t.UserAgent = &userAgent
	}
	if err := s.tokens.Insert(ctx, t); err != nil {
		return "", err
	}
	return raw, nil
}

// purgeDeadTokens deletes this user's rows that expired/were revoked longer
// than the retention window ago. Best-effort bookkeeping on the login path;
// row counts are tiny, so per-row deletes keep the SQL engine-agnostic.
func (s *Service) purgeDeadTokens(ctx context.Context, userID vo.Id, now time.Time) error {
	for _, kind := range []string{model.TokenKindSession, model.TokenKindPersonal} {
		rows, err := s.tokens.ListByUser(ctx, userID, kind)
		if err != nil {
			return err
		}
		for i := range rows {
			if rows[i].IsDead(now, deadTokenRetention) {
				if err := s.tokens.Delete(ctx, rows[i].ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
