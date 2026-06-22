package cli

import (
	"log/slog"
	"os"
)

// ConfigureLogging parses Symfony-style verbosity flags out of args, configures
// the default slog logger's level accordingly, and returns args with those flags
// removed (so commands see only their own arguments). It is called once, before
// the command runs, so the chosen level also governs startup logs (.env load,
// database open, etc.).
//
// Mapping (matching Symfony Console verbosity):
//
//	(none)         -> WARN   (quiet by default; only warnings/errors)
//	-v|--verbose   -> INFO
//	-vv            -> DEBUG
//	-vvv           -> DEBUG + source locations
//	-q|--quiet     -> silence (suppresses all log records)
//
// Verbosity flags are additive, so `-v -v` == `-vv`. -q wins over any -v.
func ConfigureLogging(args []string) []string {
	v, quiet, rest := parseVerbosity(args)

	opts := &slog.HandlerOptions{Level: levelFor(v, quiet)}
	if !quiet && v >= 3 {
		opts.AddSource = true
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
	return rest
}

// parseVerbosity returns the accumulated verbosity count, whether quiet was
// requested, and args with the recognized verbosity/quiet flags removed.
func parseVerbosity(args []string) (verbosity int, quiet bool, rest []string) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "-v", "--verbose":
			verbosity++
		case "-vv":
			verbosity += 2
		case "-vvv":
			verbosity += 3
		case "-q", "--quiet":
			quiet = true
		default:
			rest = append(rest, a)
		}
	}
	return verbosity, quiet, rest
}

// levelFor maps a verbosity count (and the quiet flag) to a slog level.
func levelFor(verbosity int, quiet bool) slog.Level {
	if quiet {
		// Above every level we emit (Error == 8), so nothing is logged.
		return slog.LevelError + 4
	}
	switch {
	case verbosity <= 0:
		return slog.LevelWarn
	case verbosity == 1:
		return slog.LevelInfo
	default: // -vv and -vvv
		return slog.LevelDebug
	}
}
