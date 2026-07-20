package model

import "time"

// UserViewRow / OptionViewRow are the user read-side row shapes the
// user.ReadModel port returns. They are declared here, in model, rather than
// in infra, so the app layer does not import the infra package (dependency
// points inward). AccessLevel/AccessUntil are the raw stored columns; the
// service collapses them with EffectiveAccessLevel before putting them on the
// wire.
//
// AccessLevel is cast from the stored text rather than strict-parsed. The
// enforcement points live elsewhere: the auth hot path strict-parses the
// caller's own level (user/repo/accesstoken.go — an unrecognized value fails
// the request), and the middleware write gate restricts anything that is not
// AccessLevelFull. Note that EffectiveAccessLevel does NOT normalize: an
// unrecognized stored level with no elapsed expiry passes through verbatim, so
// a read model displaying a level other than the authenticated caller's own
// (as the connection owner embed does) shows the raw column value.
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
