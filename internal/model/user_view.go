package model

import "time"

// UserViewRow / OptionViewRow are the user read-side row shapes the
// user.ReadModel port returns. They are declared here, in model, rather than
// in infra, so the app layer does not import the infra package (dependency
// points inward). AccessLevel/AccessUntil are the raw stored columns; the
// service collapses them with EffectiveAccessLevel before putting them on the
// wire.
type UserViewRow struct {
	ID          string
	Email       string
	Name        string
	Avatar      string
	AccessLevel string
	AccessUntil *time.Time
}

type OptionViewRow struct {
	Name  string
	Value *string
}
