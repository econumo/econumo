package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"
)

// userCommands returns the user-management subcommands (ports of the PHP
// CreateUserCommand / ChangeUserEmailCommand / ChangeUserPasswordCommand and the
// EconumoCloudBundle ActivateUserCommand / DeactivateUsersCommand).
func userCommands() []command {
	return []command{
		{
			name:    "app:create-user",
			summary: "Create a user: app:create-user <name> <email> <password>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 3 {
					return usageErr("app:create-user <name> <email> <password>")
				}
				name := strings.TrimSpace(args[0])
				id, err := c.user.AdminCreateUser(ctx, name, strings.TrimSpace(args[1]), strings.TrimSpace(args[2]))
				if err != nil {
					return err
				}
				fmt.Printf("User %q created (id: %s)\n", name, id.String())
				return nil
			},
		},
		{
			name:    "app:change-user-email",
			summary: "Change a user's email: app:change-user-email <old-email> <new-email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 2 {
					return usageErr("app:change-user-email <old-email> <new-email>")
				}
				oldEmail, newEmail := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
				if err := c.user.AdminChangeEmail(ctx, oldEmail, newEmail); err != nil {
					return err
				}
				fmt.Printf("Email changed: %s -> %s\n", oldEmail, newEmail)
				return nil
			},
		},
		{
			name:    "app:change-user-password",
			summary: "Change a user's password: app:change-user-password <email> <password>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 2 {
					return usageErr("app:change-user-password <email> <password>")
				}
				email := strings.TrimSpace(args[0])
				if err := c.user.AdminChangePassword(ctx, email, strings.TrimSpace(args[1])); err != nil {
					return err
				}
				fmt.Printf("Password changed for %s\n", email)
				return nil
			},
		},
		{
			name:    "app:activate-user",
			summary: "Activate a user: app:activate-user <email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 1 {
					return usageErr("app:activate-user <email>")
				}
				email := strings.TrimSpace(args[0])
				if err := c.user.AdminActivate(ctx, email); err != nil {
					return err
				}
				fmt.Printf("User %s activated\n", email)
				return nil
			},
		},
		{
			name:    "app:deactivate-users",
			summary: "Deactivate users created before a date: app:deactivate-users --date=YYYY-MM-DD",
			run: func(ctx context.Context, c *container, args []string) error {
				fs := flag.NewFlagSet("app:deactivate-users", flag.ContinueOnError)
				var dateStr string
				fs.StringVar(&dateStr, "date", "", "cutoff date (YYYY-MM-DD); users created before it are deactivated")
				fs.StringVar(&dateStr, "d", "", "alias for --date")
				if err := fs.Parse(args); err != nil {
					return err
				}
				if strings.TrimSpace(dateStr) == "" {
					return usageErr("app:deactivate-users --date=YYYY-MM-DD")
				}
				cutoff, err := time.Parse("2006-01-02", strings.TrimSpace(dateStr))
				if err != nil {
					return fmt.Errorf("invalid --date %q (want YYYY-MM-DD): %w", dateStr, err)
				}
				n, err := c.user.AdminDeactivateOlderThan(ctx, cutoff)
				if err != nil {
					return err
				}
				fmt.Printf("Deactivated %d user(s) created before %s\n", n, cutoff.Format("2006-01-02"))
				return nil
			},
		},
	}
}
