package currency

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/econumo/econumo/internal/model"
)

// FindRateGaps lists the periods in [from, to] where a currency in use has no
// rate against base. A zero from starts at the oldest transaction (today when
// there are none), a zero to ends today, and a to past today is clamped to
// today: future rates do not exist yet.
func (s *WriteService) FindRateGaps(ctx context.Context, base string, period model.RatePeriod, from, to time.Time) (model.RateGapReport, error) {
	base = normalizeCode(base)
	codes, err := s.write.CurrencyCodes(ctx)
	if err != nil {
		return model.RateGapReport{}, err
	}
	baseID, ok := codes[base]
	if !ok {
		return model.RateGapReport{}, fmt.Errorf("base currency %s is not a known currency", base)
	}

	today := periodStart(s.clock.Now(), model.RatePeriodDay)
	if from.IsZero() {
		first, ok, err := s.write.EarliestTransactionDate(ctx)
		if err != nil {
			return model.RateGapReport{}, err
		}
		from = today
		if ok {
			from = first
		}
	}
	if to.IsZero() || to.After(today) {
		to = today
	}
	from, to = periodStart(from, period), periodStart(to, period)
	if from.After(to) {
		return model.RateGapReport{}, fmt.Errorf("start %s is after end %s", from.Format("2006-01-02"), to.Format("2006-01-02"))
	}

	used, err := s.write.UsedCurrencyCodes(ctx)
	if err != nil {
		return model.RateGapReport{}, err
	}
	used = slices.DeleteFunc(used, func(c string) bool { return c == base })
	report := model.RateGapReport{Base: base, Period: period, From: from, To: to, Currencies: used}
	if len(used) == 0 {
		return report, nil
	}

	rated, err := s.write.RatedPeriods(ctx, baseID, period, from, nextPeriod(to, period))
	if err != nil {
		return model.RateGapReport{}, err
	}
	have := make(map[model.RatedPeriod]bool, len(rated))
	for _, r := range rated {
		have[model.RatedPeriod{Start: r.Start.UTC(), Code: r.Code}] = true
	}
	for p := from; !p.After(to); p = nextPeriod(p, period) {
		var missing []string
		for _, code := range used {
			if !have[model.RatedPeriod{Start: p, Code: code}] {
				missing = append(missing, code)
			}
		}
		if len(missing) > 0 {
			report.Gaps = append(report.Gaps, model.RateGap{Start: p, Missing: missing})
		}
	}
	return report, nil
}

// BackfillRates loads the missing rates of every gap in the report, one
// loader request per gap, dated the gap's first day. It stops at the first
// failure; the gaps already filled stay stored, so running it again resumes.
func (s *WriteService) BackfillRates(ctx context.Context, loader RateLoader, report model.RateGapReport, progress func(model.RateBackfill)) error {
	for _, gap := range report.Gaps {
		rates, err := loader.Load(ctx, gap.Start, report.Base, gap.Missing)
		if err != nil {
			return fmt.Errorf("load rates for %s: %w", gap.Start.Format("2006-01-02"), err)
		}
		returned := make(map[string]bool, len(rates))
		for i := range rates {
			returned[normalizeCode(rates[i].Code)] = true
			// The loader dates rates by the response timestamp; the file for a
			// past day is that day's rate, so it is filed under the day asked
			// for, or a timestamp past midnight UTC would leave the gap open.
			rates[i].Date = gap.Start
		}
		stored, err := s.UpdateRates(ctx, rates)
		if err != nil {
			return fmt.Errorf("store rates for %s: %w", gap.Start.Format("2006-01-02"), err)
		}
		result := model.RateBackfill{Date: gap.Start, Requested: gap.Missing, Stored: stored}
		for _, code := range gap.Missing {
			if !returned[code] {
				result.Unavailable = append(result.Unavailable, code)
			}
		}
		if progress != nil {
			progress(result)
		}
	}
	return nil
}

func periodStart(t time.Time, period model.RatePeriod) time.Time {
	t = t.UTC()
	if period == model.RatePeriodDay {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func nextPeriod(t time.Time, period model.RatePeriod) time.Time {
	if period == model.RatePeriodDay {
		return t.AddDate(0, 0, 1)
	}
	return t.AddDate(0, 1, 0)
}
