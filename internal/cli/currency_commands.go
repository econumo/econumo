package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// currencyCommands returns the currency-management subcommands (ports of the PHP
// UpdateCurrencyRatesCommand, AddCurrencyCommand, and
// RestoreCurrencyFractionDigitsCommand).
func currencyCommands() []command {
	return []command{
		{
			name:    "currency:update-rates",
			summary: "Load exchange rates from Open Exchange Rates: currency:update-rates [YYYY-MM-DD]",
			run: func(ctx context.Context, c *container, args []string) error {
				date := c.clk.Now()
				// The single optional positional is the date. Ignore any leading-dash
				// tokens (e.g. a Symfony-style -vvv/-q/-n carried over by muscle memory)
				// so they don't get misread as the date.
				if arg := firstPositional(args); arg != "" {
					d, err := time.Parse("2006-01-02", arg)
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
					fd, err := strconv.Atoi(strings.TrimSpace(args[2]))
					if err != nil {
						return fmt.Errorf("invalid fraction-digits %q: %w", args[2], err)
					}
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
