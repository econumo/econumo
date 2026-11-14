package ai

import "testing"

func TestKeysMapBack(t *testing.T) {
	k := NewKeys("a")
	id1, id2 := "019a0000-0000-7000-8000-000000000001", "019a0000-0000-7000-8000-000000000002"
	if k.Key(id1) != "a1" || k.Key(id2) != "a2" || k.Key(id1) != "a1" {
		t.Fatal("keys are not a1, a2 and stable")
	}
	for key, want := range map[string]string{"a1": id1, " A2 ": id2} {
		if got, ok := k.ID(key); !ok || got != want {
			t.Fatalf("ID(%q) = %q, %v", key, got, ok)
		}
	}
	for _, unknown := range []string{"a3", "c1", id1, ""} {
		if _, ok := k.ID(unknown); ok {
			t.Fatalf("ID(%q) resolved", unknown)
		}
	}
}
