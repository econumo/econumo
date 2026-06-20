package migrate

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// nowUTC returns the current time in UTC, used for the applied_at column.
func nowUTC() time.Time {
	return time.Now().UTC()
}

// placeholderStyle selects the bound-parameter syntax for the driver currently
// in use. The runner supports two pure-Go drivers with incompatible placeholder
// conventions:
//
//	modernc.org/sqlite -> positional "?"
//	lib/pq (postgres)  -> ordinal "$1", "$2", ...
//
// The style is detected by inspecting the driver's type name (set by the chosen
// Backend at Open time) rather than threading dialect state through Run, keeping
// Run's signature minimal. Detection is only needed for the trivial
// schema_migrations INSERT; all real schema/query SQL lives in the embedded
// migrations and sqlc-generated code.
var pgDriver = func(driverType string) bool {
	t := strings.ToLower(driverType)
	return strings.Contains(t, "pq") || strings.Contains(t, "postgres")
}

// placeholdersFor renders n placeholders ("?,?" or "$1,$2") for the given
// driver value (typically the result of (*sql.DB).Driver()).
func placeholdersFor(driver any, n int) string {
	usePg := false
	if driver != nil {
		usePg = pgDriver(reflect.TypeOf(driver).String())
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		if usePg {
			parts[i] = fmt.Sprintf("$%d", i+1)
		} else {
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ", ")
}
