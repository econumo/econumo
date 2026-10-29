package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

// currencyCommands returns the currency-management subcommands.
func currencyCommands() []command {
	return []command{
		{
			name:    "currency:update-rates",
			summary: "Load exchange rates from Open Exchange Rates: currency:update-rates [YYYY-MM-DD]",
			run: func(ctx context.Context, c *container, args []string) error {
				date := c.clk.Now()
				// The single optional positional is the date; firstPositional skips
				// stray leading-dash flags so they aren't misread as the date.
				if arg := firstPositional(args); arg != "" {
					d, err := time.Parse(datetime.DateLayout, arg)
					if err != nil {
						return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", arg, err)
					}
					date = d
				}
				codes, err := c.currency.AvailableCodes(ctx)
				if err != nil {
					return err
				}
				rates, err := c.loader.Load(ctx, date, c.cfg.CurrencyBase, codes)
				if err != nil {
					return err
				}
				n, err := c.currency.UpdateRates(ctx, rates)
				if err != nil {
					return err
				}
				fmt.Printf("Loaded %d rate(s); updated %d\n", len(rates), n)
				return nil
			},
		},
		{
			name:    "currency:show-rate-gaps",
			summary: "List months (or days, --daily) without a rate for a currency in use: currency:show-rate-gaps [start] [end] [--daily]",
			run: func(ctx context.Context, c *container, args []string) error {
				const usage = "currency:show-rate-gaps [start] [end] [--daily]"
				a, err := parseRateGapArgs(args, usage)
				if err != nil {
					return err
				}
				report, err := c.currency.FindRateGaps(ctx, c.cfg.CurrencyBase, a.period, a.from, a.to)
				if err != nil {
					return err
				}
				printRateGaps(os.Stdout, report)
				return nil
			},
		},
		{
			name:    "currency:backfill-rates",
			summary: "Load the missing rates from Open Exchange Rates, one request per month (or per day, --daily): currency:backfill-rates <start> [end] [--daily]",
			run: func(ctx context.Context, c *container, args []string) error {
				const usage = "currency:backfill-rates <start> [end] [--daily]"
				a, err := parseRateGapArgs(args, usage)
				if err != nil {
					return err
				}
				// Each gap costs one API request, so the start is required: a
				// default reaching back to the first transaction could spend a
				// month of quota in one --daily run.
				if a.from.IsZero() {
					return usageErr(usage)
				}
				report, err := c.currency.FindRateGaps(ctx, c.cfg.CurrencyBase, a.period, a.from, a.to)
				if err != nil {
					return err
				}
				if len(report.Gaps) == 0 {
					printRateGaps(os.Stdout, report)
					return nil
				}
				fmt.Printf("Loading rates against %s for %d %s (one request each)\n", report.Base, len(report.Gaps), periodNoun(report.Period, len(report.Gaps)))
				return c.currency.BackfillRates(ctx, c.loader, report, func(r model.RateBackfill) {
					printRateBackfill(os.Stdout, r)
				})
			},
		},
		{
			name:    "currency:add",
			summary: "Add a currency: currency:add <code> [name] [fraction-digits]",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) < 1 || len(args) > 3 {
					return usageErr("currency:add <code> [name] [fraction-digits]")
				}
				code := strings.TrimSpace(args[0])

				var namePtr *string
				if len(args) >= 2 && strings.TrimSpace(args[1]) != "" {
					name := strings.TrimSpace(args[1])
					namePtr = &name
				}
				var fdPtr *int
				if len(args) == 3 && strings.TrimSpace(args[2]) != "" {
					parsed, err := strconv.ParseInt(strings.TrimSpace(args[2]), 10, 16)
					if err != nil {
						return fmt.Errorf("invalid fraction-digits %q: %w", args[2], err)
					}
					fd := int(parsed)
					fdPtr = &fd
				}

				created, err := c.currency.AddCurrency(ctx, code, namePtr, fdPtr)
				if err != nil {
					return err
				}
				if created {
					fmt.Printf("Currency %s added\n", strings.ToUpper(code))
				} else {
					fmt.Printf("Currency %s already exists\n", strings.ToUpper(code))
				}
				return nil
			},
		},
	}
}

type rateGapArgs struct {
	from, to time.Time
	period   model.RatePeriod
}

// parseRateGapArgs reads up to two dates (YYYY-MM-DD or YYYY-MM) and the
// --daily flag. Single-dash arguments are the global log flags (-v, -q) and are
// skipped; any other double-dash flag is refused so a typo cannot silently
// switch the granularity.
func parseRateGapArgs(args []string, usage string) (rateGapArgs, error) {
	var a rateGapArgs
	var dates []time.Time
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		switch {
		case arg == "":
		case arg == "--daily":
			a.period = model.RatePeriodDay
		case strings.HasPrefix(arg, "--"):
			return a, usageErr(usage)
		case strings.HasPrefix(arg, "-"):
		default:
			d, err := parseRateGapDate(arg)
			if err != nil {
				return a, err
			}
			dates = append(dates, d)
		}
	}
	if len(dates) > 2 {
		return a, usageErr(usage)
	}
	if len(dates) > 0 {
		a.from = dates[0]
	}
	if len(dates) > 1 {
		a.to = dates[1]
	}
	return a, nil
}

func parseRateGapDate(s string) (time.Time, error) {
	if d, err := time.Parse(datetime.DateLayout, s); err == nil {
		return d, nil
	}
	if d, err := time.Parse("2006-01", s); err == nil {
		return d, nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD or YYYY-MM)", s)
}

func periodLabel(p model.RatePeriod, t time.Time) string {
	if p == model.RatePeriodDay {
		return t.Format(datetime.DateLayout)
	}
	return t.Format("2006-01")
}

func periodNoun(p model.RatePeriod, n int) string {
	switch {
	case p == model.RatePeriodDay && n == 1:
		return "day"
	case p == model.RatePeriodDay:
		return "days"
	case n == 1:
		return "month"
	default:
		return "months"
	}
}

func printRateGaps(w io.Writer, r model.RateGapReport) {
	span := periodLabel(r.Period, r.From) + " to " + periodLabel(r.Period, r.To)
	if len(r.Currencies) == 0 {
		fmt.Fprintf(w, "No currency other than %s is in use, so no rates are needed (%s)\n", r.Base, span)
		return
	}
	fmt.Fprintf(w, "Rates against %s for %s, %s\n", r.Base, strings.Join(r.Currencies, ", "), span)
	if len(r.Gaps) == 0 {
		fmt.Fprintln(w, "No gaps")
		return
	}
	for _, g := range r.Gaps {
		fmt.Fprintf(w, "%s  missing %s\n", periodLabel(r.Period, g.Start), strings.Join(g.Missing, ", "))
	}
	fmt.Fprintf(w, "%d %s with gaps\n", len(r.Gaps), periodNoun(r.Period, len(r.Gaps)))
}

func printRateBackfill(w io.Writer, r model.RateBackfill) {
	fmt.Fprintf(w, "%s  stored %d rate(s)", r.Date.Format(datetime.DateLayout), r.Stored)
	if len(r.Unavailable) > 0 {
		fmt.Fprintf(w, "; no rate returned for %s", strings.Join(r.Unavailable, ", "))
	}
	fmt.Fprintln(w)
}
