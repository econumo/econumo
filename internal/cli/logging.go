package cli

import (
	"os"

	"github.com/econumo/econumo/internal/logging"
)

// ConfigureLogging applies the ECONUMO_LOG_LEVEL baseline plus verbosity flags
// to the default slog logger and returns args with the recognized
// verbosity/quiet flags removed (so commands see only their own arguments). It
// is called once, before the command runs, so the chosen level also governs
// startup logs (.env load, database open, etc.).
//
// The baseline is ECONUMO_LOG_LEVEL (default info); any -v/-vv/-vvv flag raises
// it to DEBUG and -q silences output. See internal/logging for the full resolution.
func ConfigureLogging(args []string) []string {
	return logging.Setup(os.Getenv("ECONUMO_LOG_LEVEL"), args)
}
