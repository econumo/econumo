package sqliteimport

import (
	"reflect"
	"strings"
	"testing"
)

func TestTopoSort_ParentsBeforeChildren(t *testing.T) {
	// transactions -> accounts -> currencies; accounts -> users; transactions -> users
	nodes := []string{"transactions", "accounts", "currencies", "users"}
	deps := map[string][]string{
		"accounts":     {"currencies", "users"},
		"transactions": {"accounts", "users"},
	}
	got, err := topoSort(nodes, deps)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	pos := map[string]int{}
	for i, n := range got {
		pos[n] = i
	}
	if len(got) != len(nodes) {
		t.Fatalf("expected %d nodes, got %d (%v)", len(nodes), len(got), got)
	}
	for child, parents := range deps {
		for _, p := range parents {
			if pos[p] > pos[child] {
				t.Errorf("parent %q (%d) must precede child %q (%d): %v", p, pos[p], child, pos[child], got)
			}
		}
	}
}

func TestTopoSort_Deterministic(t *testing.T) {
	nodes := []string{"b", "a", "c"}
	got1, _ := topoSort(nodes, nil)
	got2, _ := topoSort(nodes, nil)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatalf("not deterministic: %v vs %v", got1, got2)
	}
	if !reflect.DeepEqual(got1, []string{"a", "b", "c"}) {
		t.Fatalf("expected sorted order with no deps, got %v", got1)
	}
}

func TestTopoSort_CycleErrors(t *testing.T) {
	nodes := []string{"a", "b"}
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	_, err := topoSort(nodes, deps)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestTopoSort_EdgeToUnknownNodeIgnored(t *testing.T) {
	// A parent that is not in the node set (e.g. an excluded table) is skipped.
	nodes := []string{"a"}
	deps := map[string][]string{"a": {"excluded"}}
	got, err := topoSort(nodes, deps)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("expected [a], got %v", got)
	}
}
