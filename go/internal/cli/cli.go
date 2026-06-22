// Package cli is the command-line shell for the econumo binary: it ports the
// operational `bin/console app:*` commands (user + currency management) from the
// Symfony app. The cmd/econumo binary routes a non-flag first argument here
// (see cmd/econumo/main.go); a build-time symlink bin/console -> /econumo makes
// the legacy `bin/console <command>` invocation work inside the distroless image.
//
// It is deliberately stdlib-only (no cobra), matching the rest of the codebase.
// Command implementations live in user_commands.go and currency_commands.go; the
// shared service wiring (the composition root) is in container.go.
package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
)

// command is one CLI subcommand. name is the exact Symfony command string (e.g.
// "app:create-user"); run receives the already-built container and the args
// following the command name.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, c *container, args []string) error
}

// commandList returns every registered command, grouped by domain in sibling
// files. Building it as a function (rather than init() into a global) keeps
// ordering explicit and the registry easy to test.
func commandList() []command {
	var cs []command
	cs = append(cs, userCommands()...)
	cs = append(cs, currencyCommands()...)
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
	c, err := newContainer(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer c.Close()

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

// printUsage writes the command list (sorted by name) with one-line summaries.
func printUsage(w *os.File) {
	fmt.Fprintln(w, "Usage: bin/console <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Available commands:")
	cs := commandList()
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	for _, c := range cs {
		fmt.Fprintf(w, "  %-40s %s\n", c.name, c.summary)
	}
}

// usageErr formats a usage error for a command (exit code 1; the message tells
// the operator the correct invocation).
func usageErr(usage string) error {
	return fmt.Errorf("usage: bin/console %s", usage)
}
