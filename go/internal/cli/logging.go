package cli

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// ConfigureLogging applies Symfony-style verbosity to the default slog logger and
// returns args with the recognized verbosity/quiet flags removed (so commands see
// only their own arguments). It is called once, before the command runs, so the
// chosen level also governs startup logs (.env load, database open, etc.).
//
// Inputs match Symfony Console exactly (see Application::configureIO):
//
//	-q | --quiet                       -> QUIET   (silence all log records)
//	-v | --verbose | --verbose=1       -> VERBOSE (INFO)
//	-vv | --verbose=2                  -> VERY VERBOSE (DEBUG)
//	-vvv | --verbose=3                 -> DEBUG (DEBUG + source)
//	SHELL_VERBOSITY env (-1,1,2,3)     -> baseline, OVERRIDDEN by any flag above
//
// Like Symfony, the flags are priority-based, not additive: the highest verbosity
// flag present wins (`-v -v` == verbose, not very-verbose), and -q beats any -v.
func ConfigureLogging(args []string) []string {
	level, quiet, rest := resolveVerbosity(args, os.Getenv("SHELL_VERBOSITY"))

	opts := &slog.HandlerOptions{Level: levelFor(level, quiet)}
	if !quiet && level >= 3 {
		opts.AddSource = true
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
	return rest
}

// resolveVerbosity computes the verbosity level (0..3) and quiet flag from the
// SHELL_VERBOSITY env baseline overridden by command-line flags (Symfony
// precedence), and returns args with the recognized flags removed.
func resolveVerbosity(args []string, shellVerbosity string) (level int, quiet bool, rest []string) {
	// Baseline from SHELL_VERBOSITY (Symfony: -1 quiet, 1 verbose, 2 very, 3 debug).
	switch n, _ := strconv.Atoi(strings.TrimSpace(shellVerbosity)); n {
	case -1:
		quiet = true
	case 1, 2, 3:
		level = n
	}

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
			// Any other --verbose=<x> means "verbose" (Symfony treats a truthy
			// --verbose value as at least VERBOSE).
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
		return level, quiet, rest // no flags: keep the env baseline
	}
}

// levelFor maps a verbosity level (0..3) and the quiet flag to a slog level.
func levelFor(level int, quiet bool) slog.Level {
	if quiet {
		// Above every level we emit (Error == 8), so nothing is logged.
		return slog.LevelError + 4
	}
	switch {
	case level <= 0:
		return slog.LevelWarn
	case level == 1:
		return slog.LevelInfo
	default: // 2 (very verbose) and 3 (debug)
		return slog.LevelDebug
	}
}
