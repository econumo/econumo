package repo

import "testing"

func TestHydrate_InvalidIDs(t *testing.T) {
	if _, err := hydrate(labelRow{ID: "not-a-uuid", UserID: "22222222-2222-2222-2222-222222222222"}); err == nil {
		t.Fatal("want error for an invalid label id")
	}
	if _, err := hydrate(labelRow{ID: "11111111-1111-1111-1111-111111111111", UserID: "not-a-uuid"}); err == nil {
		t.Fatal("want error for an invalid user id")
	}
}
