// Package sqliteimport copies an Econumo SQLite database into an
// already-migrated PostgreSQL database. It introspects the target schema for the
// table list, column types, and foreign-key order, so it carries no hardcoded
// schema and does not drift as migrations evolve.
package sqliteimport

import (
	"errors"
	"fmt"
	"sort"
)

// ErrTargetNotEmpty is returned by Import when the target already holds user
// data and force is false; nothing is copied in that case.
var ErrTargetNotEmpty = errors.New("target database already contains data")

type TableCount struct {
	Name string
	Rows int64
}

type Report struct {
	Tables []TableCount
	Total  int64
}

// topoSort orders nodes so every table precedes the tables that reference it.
// deps[child] lists child's referenced (parent) tables; edges to names outside
// the node set are ignored (e.g. FKs into excluded tables). Order is
// deterministic. A foreign-key cycle is an error.
func topoSort(nodes []string, deps map[string][]string) ([]string, error) {
	inSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		inSet[n] = true
	}
	sorted := append([]string(nil), nodes...)
	sort.Strings(sorted)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(nodes))
	var order []string

	var visit func(n string) error
	visit = func(n string) error {
		switch color[n] {
		case black:
			return nil
		case gray:
			return fmt.Errorf("sqliteimport: foreign-key cycle at table %q", n)
		}
		color[n] = gray
		parents := append([]string(nil), deps[n]...)
		sort.Strings(parents)
		for _, p := range parents {
			if p == n || !inSet[p] {
				continue
			}
			if err := visit(p); err != nil {
				return err
			}
		}
		color[n] = black
		order = append(order, n)
		return nil
	}
	for _, n := range sorted {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}
