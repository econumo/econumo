package model

import "time"

// TrialEnd returns the instant a trial of the given number of days ends,
// relative to the registration time (in UTC, since access expiry is compared
// against a UTC clock).
func TrialEnd(registeredAt time.Time, days int) time.Time {
	return registeredAt.UTC().AddDate(0, 0, days)
}
