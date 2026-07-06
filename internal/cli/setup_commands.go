package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/shared/jwt"
)

// setupCommands returns deployment-setup subcommands. Unlike the management
// commands these do NOT need the database (noContainer), so they work on a fresh
// host before any DB exists. They read only their own environment variables.
func setupCommands() []command {
	return []command{
		{
			name:        "jwt:generate",
			summary:     "Generate the RS256 JWT keypair, skipping if present (--force overwrites)",
			noContainer: true,
			run: func(_ context.Context, _ *container, args []string) error {
				force := false
				for _, a := range args {
					if a == "--force" || a == "-f" {
						force = true
					}
				}
				privPath := config.ResolveProjectDir(envOr("ECONUMO_JWT_PRIVATE_KEY_PATH", "var/jwt/private.pem"))
				pubPath := config.ResolveProjectDir(envOr("ECONUMO_JWT_PUBLIC_KEY_PATH", "var/jwt/public.pem"))
				passphrase := os.Getenv("ECONUMO_JWT_PASSPHRASE")

				// Same shared path the server runs on boot: skip when a keypair
				// already exists, generate when missing, regenerate with --force. A
				// passphrase is auto-generated and persisted when ECONUMO_JWT_PASSPHRASE
				// is unset, so this works with zero configuration.
				_, generated, err := jwt.EnsureKeypair(privPath, pubPath, passphrase, force)
				if err != nil {
					return err
				}
				if !generated {
					fmt.Printf("JWT keypair already exists, skipped (use --force to overwrite):\n  private key: %s\n  public key:  %s\n", privPath, pubPath)
					return nil
				}
				fmt.Printf("Generated RS256 JWT keypair:\n  private key: %s\n  public key:  %s\n", privPath, pubPath)
				fmt.Println("Ensure the server runs with matching ECONUMO_JWT_PRIVATE_KEY_PATH, ECONUMO_JWT_PUBLIC_KEY_PATH and ECONUMO_JWT_PASSPHRASE.")
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
