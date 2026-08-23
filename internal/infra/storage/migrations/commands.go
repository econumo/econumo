package migrations

import "strings"

type commandStep struct{ version, name string }

var registry []commandStep

// RegisterCommand adds a command migration step: at Version, the boot runner
// invokes the CLI command `name` (must be `migration:<slug>`). Called from
// init() next to the SQL files so the ordered list stays in one place.
func RegisterCommand(version, name string) {
	if !strings.HasPrefix(name, "migration:") {
		panic("migrations: command step " + version + " must be a migration:* command, got " + name)
	}
	for _, c := range registry {
		if c.version == version {
			panic("migrations: duplicate command step version " + version)
		}
	}
	registry = append(registry, commandStep{version: version, name: name})
}

func commandSteps() []commandStep { return registry }
