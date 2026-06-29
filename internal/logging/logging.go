// Package logging is the single source of truth for configuring the default
// slog logger from a base level (ECONUMO_LOG_LEVEL / config) combined with
// verbosity flags (-v/-vv/-vvv/-q). It is reused by both the server (`serve`)
// and the management CLI so every command shares one level-resolution scheme
// and one handler setup.
//
// Resolution order (highest priority wins):
//
//	-q / --quiet                  -> silence everything
//	-v / -vv / -vvv               -> DEBUG (-vvv also enables source locations)
//	otherwise                     -> the baseline level (ECONUMO_LOG_LEVEL, default info)
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// ParseLevel maps a level name (case-insensitive) to a slog.Level. Unknown or
// empty input falls back to INFO — the default level for the app.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default: // "info" and anything unrecognized
		return slog.LevelInfo
	}
}

// Setup resolves the effective level from the baseline (an ECONUMO_LOG_LEVEL-style
// name) combined with the verbosity/quiet flags in args, installs it as the
// default slog text handler on stderr, and returns args with the recognized
// verbosity/quiet flags removed (so a command sees only its own arguments).
func Setup(baseline string, args []string) []string {
	level, quiet, rest := resolveVerbosity(args)
	lvl, addSource := effectiveLevel(ParseLevel(baseline), level, quiet)
	opts := &slog.HandlerOptions{Level: lvl, AddSource: addSource}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
	return rest
}

// resolveVerbosity computes the verbosity level (0..3) and quiet flag from the
// command-line flags, and returns args with the recognized flags removed:
//
//	-q | --quiet                   -> QUIET
//	-v | --verbose | --verbose=1   -> 1
//	-vv | --verbose=2              -> 2
//	-vvv | --verbose=3             -> 3
//
// Flags are priority-based, not additive: the highest verbosity flag present
// wins (`-v -v` == 1, not 2), and -q beats any -v.
func resolveVerbosity(args []string) (level int, quiet bool, rest []string) {
	var hasQuiet, hasV1, hasV2, hasV3 bool
	rest = make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "-q" || a == "--quiet":
			hasQuiet = true
		case a == "-vvv" || a == "--verbose=3":
			hasV3 = true
		case a == "-vv" || a == "--verbose=2":
			hasV2 = true
		case a == "-v" || a == "--verbose" || a == "--verbose=1":
			hasV1 = true
		case strings.HasPrefix(a, "--verbose="):
			// Any other --verbose=<x> counts as at least level 1.
			hasV1 = true
		default:
			rest = append(rest, a)
			continue
		}
	}

	// Flags override the env baseline, highest wins; -q beats everything.
	switch {
	case hasQuiet:
		return 0, true, rest
	case hasV3:
		return 3, false, rest
	case hasV2:
		return 2, false, rest
	case hasV1:
		return 1, false, rest
	default:
		return 0, false, rest // no flags: use the baseline level
	}
}

// effectiveLevel combines the baseline level with the resolved verbosity. Any
// verbose flag drops to DEBUG — since the default is already INFO there is no
// separate INFO step; -vvv additionally enables source locations. quiet silences
// everything (a level above ERROR).
func effectiveLevel(baseline slog.Level, level int, quiet bool) (lvl slog.Level, addSource bool) {
	switch {
	case quiet:
		return slog.LevelError + 4, false
	case level >= 1:
		return slog.LevelDebug, level >= 3
	default:
		return baseline, false
	}
}
