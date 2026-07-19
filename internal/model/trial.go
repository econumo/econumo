package model

import "time"

// TrialEnd returns the first instant of the month after the registration month.
// The product's moment of value is a closed calendar month (plan against
// actual), so the trial must span one whole month whatever day it starts on: a
// fixed day count delivers that to nobody registering early in a month. Taking
// the start of the following month rather than the last second of the previous
// one avoids end-of-day arithmetic and leaves timezone slack.
func TrialEnd(registeredAt time.Time) time.Time {
	utc := registeredAt.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 2, 0)
}
