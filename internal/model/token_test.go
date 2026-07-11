package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func tokenAt(kind string, exp *time.Time) *AccessToken {
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	return &AccessToken{
		ID: vo.NewId(), UserID: vo.NewId(), Kind: kind, TokenHash: "h",
		CreatedAt: base, LastUsedAt: base, ExpiresAt: exp,
	}
}

func TestAccessToken_IsLive(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if !tokenAt(TokenKindPersonal, nil).IsLive(now) {
		t.Error("nil expiry (never expires) must be live")
	}
	if !tokenAt(TokenKindSession, &future).IsLive(now) {
		t.Error("future expiry must be live")
	}
	if tokenAt(TokenKindSession, &past).IsLive(now) {
		t.Error("past expiry must be dead")
	}
	revoked := tokenAt(TokenKindPersonal, nil)
	revoked.Revoke(now)
	if revoked.IsLive(now.Add(time.Second)) {
		t.Error("revoked must be dead")
	}
}

func TestAccessToken_TouchSlidesSessionOnly(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ttl := 30 * 24 * time.Hour

	s := tokenAt(TokenKindSession, nil)
	s.Touch(now, ttl)
	if s.LastUsedAt != now {
		t.Errorf("LastUsedAt = %v, want %v", s.LastUsedAt, now)
	}
	if s.ExpiresAt == nil || !s.ExpiresAt.Equal(now.Add(ttl)) {
		t.Errorf("session ExpiresAt = %v, want %v", s.ExpiresAt, now.Add(ttl))
	}

	patExp := now.Add(48 * time.Hour)
	p := tokenAt(TokenKindPersonal, &patExp)
	p.Touch(now, ttl)
	if p.ExpiresAt == nil || !p.ExpiresAt.Equal(patExp) {
		t.Errorf("PAT expiry must not slide: got %v, want %v", p.ExpiresAt, patExp)
	}
	if p.LastUsedAt != now {
		t.Errorf("PAT LastUsedAt = %v, want %v", p.LastUsedAt, now)
	}
}

func TestAccessToken_NeedsTouch(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tok := tokenAt(TokenKindSession, nil)
	tok.LastUsedAt = now.Add(-4 * time.Minute)
	if tok.NeedsTouch(now, 5*time.Minute) {
		t.Error("4 minutes old: no touch")
	}
	tok.LastUsedAt = now.Add(-5 * time.Minute)
	if !tok.NeedsTouch(now, 5*time.Minute) {
		t.Error("5 minutes old: touch")
	}
}

func TestAccessToken_RevokeIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tok := tokenAt(TokenKindSession, nil)
	tok.Revoke(now)
	first := *tok.RevokedAt
	tok.Revoke(now.Add(time.Hour))
	if !tok.RevokedAt.Equal(first) {
		t.Error("second Revoke must not move the timestamp")
	}
}

func TestAccessToken_IsDead(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	retention := 30 * 24 * time.Hour

	live := tokenAt(TokenKindPersonal, nil)
	if live.IsDead(now, retention) {
		t.Error("live token is not dead")
	}
	oldExp := now.Add(-retention - time.Hour)
	expired := tokenAt(TokenKindSession, &oldExp)
	if !expired.IsDead(now, retention) {
		t.Error("expired past retention must be dead")
	}
	recentExp := now.Add(-time.Hour)
	recent := tokenAt(TokenKindSession, &recentExp)
	if recent.IsDead(now, retention) {
		t.Error("recently expired stays within retention")
	}
	revoked := tokenAt(TokenKindSession, nil)
	revoked.Revoke(now.Add(-retention - time.Hour))
	if !revoked.IsDead(now, retention) {
		t.Error("revoked past retention must be dead")
	}
}
