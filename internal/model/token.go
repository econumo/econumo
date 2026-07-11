package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Access-token kinds: a login session (sliding expiry) or a personal access
// token (fixed or no expiry, created explicitly by the user for integrations).
const (
	TokenKindSession  = "session"
	TokenKindPersonal = "personal"
)

// AccessToken is one opaque bearer credential. Only the sha256 hash of the
// token string is stored; the raw token exists client-side only.
type AccessToken struct {
	ID         vo.Id
	UserID     vo.Id
	Kind       string
	TokenHash  string
	Name       *string // PAT only: the user-given label
	UserAgent  *string // session only: User-Agent captured at login
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  *time.Time // nil = never expires (PAT); sessions always have one
	RevokedAt  *time.Time
}

func (t *AccessToken) IsLive(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	return t.ExpiresAt == nil || t.ExpiresAt.After(now)
}

// NeedsTouch reports whether the last-used stamp is stale enough to persist —
// the write-throttle that keeps single-writer SQLite off the hot path.
func (t *AccessToken) NeedsTouch(now time.Time, interval time.Duration) bool {
	return now.Sub(t.LastUsedAt) >= interval
}

// Touch advances the last-used stamp; sessions also slide their expiry window
// (a PAT's expiry is a promise made at creation and never moves).
func (t *AccessToken) Touch(now time.Time, sessionTTL time.Duration) {
	t.LastUsedAt = now
	if t.Kind == TokenKindSession {
		exp := now.Add(sessionTTL)
		t.ExpiresAt = &exp
	}
}

func (t *AccessToken) Revoke(now time.Time) {
	if t.RevokedAt == nil {
		t.RevokedAt = &now
	}
}

// IsDead reports whether the row has been expired/revoked for longer than the
// retention window and can be purged.
func (t *AccessToken) IsDead(now time.Time, retention time.Duration) bool {
	if t.RevokedAt != nil && now.Sub(*t.RevokedAt) > retention {
		return true
	}
	if t.ExpiresAt != nil && now.Sub(*t.ExpiresAt) > retention {
		return true
	}
	return false
}
