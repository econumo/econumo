package model

import "time"

// UserViewRow / OptionViewRow are the user read-side row shapes the
// user.ReadModel port returns. They are declared here, in model, rather than
// in infra, so the app layer does not import the infra package (dependency
// points inward). AccessLevel/AccessUntil are the raw stored columns; the
// service collapses them with EffectiveAccessLevel before putting them on the
// wire.
//
// AccessLevel is cast from the stored text rather than strict-parsed (as the
// auth hot path does in user/repo/accesstoken.go): that path is the enforcement
// point and rejects an unrecognized level outright, so no request carrying one
// ever reaches a read model. Should one slip through, EffectiveAccessLevel
// collapses anything that is not AccessLevelFull to read-only — fail-closed
// either way.
type UserViewRow struct {
	ID          string
	Email       string
	Name        string
	Avatar      string
	AccessLevel AccessLevel
	AccessUntil *time.Time
}

type OptionViewRow struct {
	Name  string
	Value *string
}
