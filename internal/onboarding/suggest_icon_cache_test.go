package onboarding

import "testing"

func TestIconCache_EvictsTheLeastRecentlyUsed(t *testing.T) {
	c := newIconCache(2)
	c.put("a", "home")
	c.put("b", "")
	if _, ok := c.get("a"); !ok {
		t.Fatal("a missing")
	}
	c.put("c", "star")
	if _, ok := c.get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if icon, ok := c.get("a"); !ok || icon != "home" {
		t.Fatalf("a = %q %v", icon, ok)
	}
	if icon, ok := c.get("c"); !ok || icon != "star" {
		t.Fatalf("c = %q %v", icon, ok)
	}
}
