package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
)

// setupCommands returns deployment-setup subcommands. Unlike the management
// commands these do NOT need the database (noContainer), so they work on a fresh
// host before any DB exists. They read only their own environment variables.
func setupCommands() []command {
	return []command{
		{
			name:        "app:generate-jwt-keypair",
			summary:     "Generate the RS256 JWT keypair (JWT_SECRET_KEY/JWT_PUBLIC_KEY)",
			noContainer: true,
			run: func(_ context.Context, _ *container, args []string) error {
				force := false
				for _, a := range args {
					if a == "--force" || a == "-f" {
						force = true
					}
				}
				privPath := config.ResolveProjectDir(envOr("JWT_SECRET_KEY", "var/jwt/private.pem"))
				pubPath := config.ResolveProjectDir(envOr("JWT_PUBLIC_KEY", "var/jwt/public.pem"))
				passphrase := os.Getenv("JWT_PASSPHRASE")

				if err := auth.GenerateKeypair(privPath, pubPath, passphrase, force); err != nil {
					return err
				}
				fmt.Printf("Generated RS256 JWT keypair:\n  private key: %s\n  public key:  %s\n", privPath, pubPath)
				fmt.Println("Ensure the server runs with matching JWT_SECRET_KEY, JWT_PUBLIC_KEY and JWT_PASSPHRASE.")
				return nil
			},
		},
	}
}

// envOr returns the environment value for key, or def when unset/empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
