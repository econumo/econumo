package user

import (
	"context"
	"encoding/json"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type regrantAuditDetails struct {
	FromUserID     string `json:"fromUserId"`
	DeletedEventID string `json:"deletedEventId"`
}

// regrantLifetime gives a newly proven address back the lifetime access its
// deleted account had, once per deletion. It runs inside the caller's
// transaction and only sets the access on u: the caller saves it.
func (s *Service) regrantLifetime(ctx context.Context, u *model.User, plainEmail string, now time.Time) (bool, error) {
	if s.audit == nil {
		return false, nil
	}
	hash := model.EmailHash(plainEmail)
	rows, err := s.audit.ListByEmailHash(ctx, hash)
	if err != nil {
		return false, err
	}
	deleted, err := openLifetimeDeletion(rows)
	if err != nil || deleted == nil {
		return false, err
	}
	raw, err := json.Marshal(regrantAuditDetails{FromUserID: deleted.UserID.String(), DeletedEventID: deleted.ID.String()})
	if err != nil {
		return false, err
	}
	ref := deleted.ID
	// The unique ref_id is what makes this once: of two accounts racing for the
	// same deletion, only one insert lands.
	inserted, err := s.audit.Insert(ctx, model.UserAuditEvent{
		ID: vo.NewId(), UserID: u.ID, Event: model.AuditLifetimeRegranted,
		EmailHash: &hash, RefID: &ref, Details: string(raw), CreatedAt: now,
	})
	if err != nil || !inserted {
		return false, err
	}
	u.SetAccess(model.AccessLevelFull, nil, now)
	return true, nil
}

// regrantOnFirstProof re-grants only on the unverified-to-verified transition
// of a newly registered account: an address that was already verified proves
// nothing new, and an address taken over by an email change is not a new
// registration.
func (s *Service) regrantOnFirstProof(ctx context.Context, u *model.User, now time.Time) error {
	if u.EmailVerified {
		return nil
	}
	plain, err := s.encode.Decode(u.Email)
	if err != nil {
		return err
	}
	_, err = s.regrantLifetime(ctx, u, plain, now)
	return err
}

// openLifetimeDeletion matches by reference rather than by time: a regrant and
// a later deletion can share a created_at second.
func openLifetimeDeletion(rows []model.UserAuditEvent) (*model.UserAuditEvent, error) {
	consumed := make(map[string]bool)
	for _, r := range rows {
		if r.Event == model.AuditLifetimeRegranted && r.RefID != nil {
			consumed[r.RefID.String()] = true
		}
	}
	for i := range rows {
		r := rows[i]
		if r.Event != model.AuditDeleted || consumed[r.ID.String()] {
			continue
		}
		var d deletedAuditDetails
		if err := json.Unmarshal([]byte(r.Details), &d); err != nil {
			return nil, err
		}
		if d.LifetimeAccess {
			return &rows[i], nil
		}
	}
	return nil, nil
}
