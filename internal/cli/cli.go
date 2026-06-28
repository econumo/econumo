// Package cli is the command-line shell for the econumo binary: the operational
// `app:*` commands (user + currency management). The cmd/econumo binary routes a
// non-flag first argument here (see cmd/econumo/main.go), so `econumo app:<cmd>`
// (or `docker exec <container> /econumo app:<cmd>` in the image) runs a command.
//
// It is deliberately stdlib-only (no cobra), matching the rest of the codebase.
// Command implementations live in user_commands.go and currency_commands.go; the
// shared service wiring (the composition root) is in container.go.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
)

// command is one CLI subcommand. name is the exact Symfony command string (e.g.
// "app:create-user"); run receives the (possibly nil) container and the args
// following the command name.
type command struct {
	name    string
	summary string
	// noContainer marks a command that does NOT need the DB-backed service
	// container (e.g. app:generate-jwt-keypair, a setup step that may run before a
	// database exists). For such commands Run passes a nil container and never
	// opens the database. Default false = the container is built.
	noContainer bool
	run         func(ctx context.Context, c *container, args []string) error
}

// commandList returns every registered command, grouped by domain in sibling
// files. Building it as a function (rather than init() into a global) keeps
// ordering explicit and the registry easy to test.
func commandList() []command {
	var cs []command
	cs = append(cs, userCommands()...)
	cs = append(cs, currencyCommands()...)
	cs = append(cs, setupCommands()...)
	return cs
}

// Run dispatches args[0] to a registered command, building the service container
// lazily (only once a real command is matched). It returns the process exit
// code: 0 success, 1 runtime error, 2 usage error (unknown/missing command).
func Run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 2
	}
	name := args[0]
	if name == "help" || name == "-h" || name == "--help" {
		printUsage(os.Stdout)
		return 0
	}

	cmds := index(commandList())
	cmd, ok := cmds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", name)
		printUsage(os.Stderr)
		return 2
	}

	ctx := context.Background()

	// Build the DB-backed container only for commands that need it; setup
	// commands (noContainer) get a nil container and never open the database.
	var c *container
	if !cmd.noContainer {
		var err error
		c, err = newContainer(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		defer c.Close()
	}

	slog.Debug("cli: running command", "command", name, "args", args[1:])
	if err := cmd.run(ctx, c, args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// index builds a name->command map, panicking on a duplicate name (a wiring
// mistake should fail loudly).
func index(cs []command) map[string]command {
	m := make(map[string]command, len(cs))
	for _, c := range cs {
		if _, dup := m[c.name]; dup {
			panic("cli: duplicate command " + c.name)
		}
		m[c.name] = c
	}
	return m
}

// printUsage writes the management-command list with a header (used when the CLI
// is invoked with no/unknown command).
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: econumo <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Available commands:")
	WriteCommandList(w)
}

// WriteCommandList writes the registered management commands (sorted by name)
// with one-line summaries. Exported so the binary's top-level usage can include
// the app:* commands alongside its own (serve, healthcheck).
func WriteCommandList(w io.Writer) {
	cs := commandList()
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	for _, c := range cs {
		fmt.Fprintf(w, "  %-40s %s\n", c.name, c.summary)
	}
}

// usageErr formats a usage error for a command (exit code 1; the message tells
// the operator the correct invocation).
func usageErr(usage string) error {
	return fmt.Errorf("usage: econumo %s", usage)
}

// firstPositional returns the first non-flag, non-empty argument (trimmed), or
// "" if there is none. It lets commands with a single optional positional ignore
// stray Symfony-style global flags (e.g. -vvv, -q, -n) carried over by habit.
func firstPositional(args []string) string {
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}
