package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const reportMonthsMax = 240

type reportWindow struct {
	from   time.Time
	months int
}

func (w reportWindow) end() time.Time { return w.from.AddDate(0, w.months, 0) }

func (w reportWindow) monthList() []time.Time {
	out := make([]time.Time, w.months)
	for i := range out {
		out[i] = w.from.AddDate(0, i, 0)
	}
	return out
}

func invalidChoice(field string) error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key: field, Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
	})
}

func parseReportMonth(raw, field string) (time.Time, bool, error) {
	if raw == "" {
		return time.Time{}, false, nil
	}
	for _, layout := range []string{datetime.Layout, datetime.DateLayout, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			y, m, _ := t.Date()
			return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC), true, nil
		}
	}
	return time.Time{}, false, invalidChoice(field)
}

// resolveReportWindow turns the raw from/to params into the month window.
// An empty from starts at the first transaction's month, an empty to ends at
// the caller's current month; endCap (an ended budget's end month) clamps to.
func (s *Service) resolveReportWindow(ctx context.Context, rawFrom, rawTo string, accountIDs []vo.Id, endCap *time.Time) (reportWindow, error) {
	from, hasFrom, err := parseReportMonth(rawFrom, "from")
	if err != nil {
		return reportWindow{}, err
	}
	to, hasTo, err := parseReportMonth(rawTo, "to")
	if err != nil {
		return reportWindow{}, err
	}
	current := localMonth(s.clock.Now(), reqctx.Location(ctx))
	if !hasTo {
		to = current
	}
	if endCap != nil && to.After(*endCap) {
		to = *endCap
	}
	if !hasFrom {
		from = current
		if len(accountIDs) > 0 {
			first, err := s.read.FirstTransactionDate(ctx, accountIDs)
			if err != nil {
				return reportWindow{}, err
			}
			if first != nil {
				from = model.FirstOfMonth(first.UTC())
			}
		}
	}
	if from.After(to) {
		from = to
	}
	months := (to.Year()-from.Year())*12 + int(to.Month()-from.Month()) + 1
	if months > reportMonthsMax {
		return reportWindow{}, invalidChoice("to")
	}
	return reportWindow{from: from, months: months}, nil
}

const (
	flowGroupCategory = "category"
	flowGroupTag      = "tag"
	flowGroupPayee    = "payee"
)

func parseFlowGroupBy(raw string) (string, error) {
	switch raw {
	case "":
		return flowGroupCategory, nil
	case flowGroupCategory, flowGroupTag, flowGroupPayee:
		return raw, nil
	}
	return "", invalidChoice("groupBy")
}
