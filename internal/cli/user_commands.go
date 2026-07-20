package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

// userCommands returns the user-management subcommands.
func userCommands() []command {
	return []command{
		{
			name:    "user:create",
			summary: "Create a user: user:create <name> <email> <password>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 3 {
					return usageErr("user:create <name> <email> <password>")
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
			name:    "user:change-email",
			summary: "Change a user's email: user:change-email <old-email> <new-email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 2 {
					return usageErr("user:change-email <old-email> <new-email>")
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
			name:    "user:change-password",
			summary: "Change a user's password: user:change-password <email> <password>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 2 {
					return usageErr("user:change-password <email> <password>")
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
			name:    "user:activate",
			summary: "Activate a user: user:activate <email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 1 {
					return usageErr("user:activate <email>")
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
			name:    "user:deactivate",
			summary: "Deactivate a user: user:deactivate <email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 1 {
					return usageErr("user:deactivate <email>")
				}
				email := strings.TrimSpace(args[0])
				if err := c.user.AdminDeactivate(ctx, email); err != nil {
					return err
				}
				fmt.Printf("User %s deactivated\n", email)
				return nil
			},
		},
		{
			name:    "user:set-access",
			summary: "Set a user's access: user:set-access <email> <full|readonly> [YYYY-MM-DD]",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) < 2 || len(args) > 3 {
					return usageErr("user:set-access <email> <full|readonly> [YYYY-MM-DD]")
				}
				email := strings.TrimSpace(args[0])
				level, err := model.ParseAccessLevel(strings.TrimSpace(args[1]))
				if err != nil {
					return err
				}
				var until *time.Time
				if len(args) == 3 && strings.TrimSpace(args[2]) != "" {
					d, err := time.Parse(datetime.DateLayout, strings.TrimSpace(args[2]))
					if err != nil {
						return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", args[2], err)
					}
					until = &d
				}
				if err := c.user.AdminSetAccess(ctx, email, level, until); err != nil {
					return err
				}
				if until == nil {
					fmt.Printf("Access for %s set to %s with no expiry\n", email, level)
				} else {
					// Access restricts once now >= until (exclusive boundary, same
					// as TrialEnd), so the resolved instant is printed rather than
					// the bare date to make that cutoff visible to the operator.
					fmt.Printf("Access for %s set to %s until %s\n", email, level, until.Format(datetime.Layout))
				}
				return nil
			},
		},
		{
			name:    "user:show",
			summary: "Show a user's profile and access: user:show <email>",
			run: func(ctx context.Context, c *container, args []string) error {
				if len(args) != 1 {
					return usageErr("user:show <email>")
				}
				u, effective, err := c.user.AdminShowUser(ctx, strings.TrimSpace(args[0]))
				if err != nil {
					return err
				}
				active := "no"
				if u.IsActive {
					active = "yes"
				}
				until := ""
				if u.AccessUntil != nil {
					until = u.AccessUntil.Format(datetime.Layout)
				}
				fmt.Printf("Id:              %s\n", u.ID.String())
				fmt.Printf("Name:            %s\n", u.Name)
				fmt.Printf("Email:           %s\n", u.Email)
				fmt.Printf("Active:          %s\n", active)
				fmt.Printf("Access level:    %s\n", u.AccessLevel)
				fmt.Printf("Access until:    %s\n", until)
				fmt.Printf("Effective:       %s\n", effective)
				return nil
			},
		},
		{
			name:    "data:remove-salt",
			summary: "Decrypt emails to plaintext + re-hash identifiers so ECONUMO_DATA_SALT can be removed",
			run: func(ctx context.Context, c *container, args []string) error {
				// Guard the catastrophic case: with an empty salt Decode is a
				// passthrough, so the sweep would store ciphertext AS plaintext.
				// The salt the data was written with MUST still be configured.
				if strings.TrimSpace(c.cfg.DataSalt) == "" {
					return errors.New("ECONUMO_DATA_SALT is empty; set it to the salt the data was written with before running this migration")
				}
				migrated, skipped, err := c.user.MigrateRemoveDataSalt(ctx, c.cfg.DataSalt)
				if err != nil {
					return err
				}
				fmt.Printf("Migrated %d user(s) to plaintext; skipped %d already-plaintext.\n", migrated, skipped)
				fmt.Println("Now remove ECONUMO_DATA_SALT from your environment and restart.")
				return nil
			},
		},
	}
}
