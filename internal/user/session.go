// Session lifecycle: creation at login and the opportunistic purge of dead
// rows.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
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

// ListSessions returns the user's LIVE sessions in repo order, marking the one
// that authenticated this request as current.
func (s *Service) ListSessions(ctx context.Context, userID, currentTokenID vo.Id) ([]model.SessionItem, error) {
	rows, err := s.tokens.ListByUser(ctx, userID, model.TokenKindSession)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make([]model.SessionItem, 0, len(rows))
	for i := range rows {
		if !rows[i].IsLive(now) {
			continue
		}
		ua := ""
		if rows[i].UserAgent != nil {
			ua = *rows[i].UserAgent
		}
		out = append(out, model.SessionItem{
			Id:         rows[i].ID.String(),
			UserAgent:  ua,
			CreatedAt:  rows[i].CreatedAt.UTC().Format(datetime.Layout),
			LastUsedAt: rows[i].LastUsedAt.UTC().Format(datetime.Layout),
			IsCurrent:  rows[i].ID.Equal(currentTokenID),
		})
	}
	return out, nil
}

// RevokeSession revokes one of the CALLER's sessions. A foreign, unknown, or
// non-session id is a uniform 404 (no existence oracle); revoking the current
// session is allowed (it is just a logout).
func (s *Service) RevokeSession(ctx context.Context, userID vo.Id, req model.RevokeSessionRequest) (*model.RevokeSessionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, errs.NewNotFound("Session not found")
	}
	t, err := s.tokens.GetByID(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, errs.NewNotFound("Session not found")
		}
		return nil, err
	}
	if !t.UserID.Equal(userID) || t.Kind != model.TokenKindSession {
		return nil, errs.NewNotFound("Session not found")
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	return &model.RevokeSessionResult{}, nil
}

// RevokeOtherSessions signs the user out everywhere except the presenting
// session.
func (s *Service) RevokeOtherSessions(ctx context.Context, userID, currentTokenID vo.Id) (*model.RevokeOtherSessionsResult, error) {
	if err := s.revokeSessions(ctx, userID, currentTokenID, s.clock.Now()); err != nil {
		return nil, err
	}
	return &model.RevokeOtherSessionsResult{}, nil
}

// revokeSessions revokes every live session of the user except exceptTokenID
// (zero id = revoke all). PATs are never touched here: integrations must
// survive a password change; only user:deactivate kills them (revokeTokens).
func (s *Service) revokeSessions(ctx context.Context, userID vo.Id, exceptTokenID vo.Id, now time.Time) error {
	return s.revokeTokens(ctx, userID, exceptTokenID, now, model.TokenKindSession)
}

func (s *Service) revokeTokens(ctx context.Context, userID vo.Id, exceptTokenID vo.Id, now time.Time, kinds ...string) error {
	for _, kind := range kinds {
		rows, err := s.tokens.ListByUser(ctx, userID, kind)
		if err != nil {
			return err
		}
		for i := range rows {
			if rows[i].ID.Equal(exceptTokenID) || !rows[i].IsLive(now) {
				continue
			}
			rows[i].Revoke(now)
			if err := s.tokens.Update(ctx, &rows[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// PurgeDeadTokens deletes EVERY user's dead rows (expired/revoked longer than
// retention ago) in one indexed DELETE — the token:purge CLI entry point. The
// per-user purgeDeadTokens below stays on the login path (opportunistic, tiny
// row counts); this is the bulk variant for operators.
func (s *Service) PurgeDeadTokens(ctx context.Context, retention time.Duration) (int64, error) {
	if retention < 0 {
		return 0, &errs.ValidationError{Msg: "Retention must not be negative", MsgCode: errs.CodeTokenRetentionNegative}
	}
	return s.tokens.DeleteDead(ctx, s.clock.Now().Add(-retention))
}

// DefaultTokenRetention is the standard dead-row retention window, shared by
// the login-path purge and the token:purge CLI default.
const DefaultTokenRetention = deadTokenRetention

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
