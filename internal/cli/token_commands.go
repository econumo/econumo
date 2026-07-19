package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appuser "github.com/econumo/econumo/internal/user"
)

// tokenCommands returns the access-token maintenance subcommands.
func tokenCommands() []command {
	return []command{
		{
			name:    "token:purge",
			summary: "Delete expired/revoked access tokens older than [days] (default 30): token:purge [days]",
			run: func(ctx context.Context, c *container, args []string) error {
				retention := appuser.DefaultTokenRetention
				if len(args) > 1 {
					return usageErr("token:purge [days]")
				}
				if len(args) == 1 {
					days, err := strconv.Atoi(strings.TrimSpace(args[0]))
					if err != nil || days < 0 {
						return usageErr("token:purge [days] (days must be a non-negative integer)")
					}
					retention = time.Duration(days) * 24 * time.Hour
				}
				n, err := c.user.PurgeDeadTokens(ctx, retention)
				if err != nil {
					return err
				}
				fmt.Printf("Purged %d dead access token(s) (expired/revoked more than %s ago).\n",
					n, formatRetention(retention))
				return nil
			},
		},
	}
}

func formatRetention(d time.Duration) string {
	days := int(d / (24 * time.Hour))
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
