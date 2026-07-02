package tag

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	v, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return v
}

var (
	tt0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	tt1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
)

func newTag(t *testing.T) *Tag {
	return NewTag(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "Travel", tt0)
}

func TestNewTag_NotArchived(t *testing.T) {
	tg := newTag(t)
	if tg.IsArchived() || tg.Name() != "Travel" {
		t.Fatalf("new tag: archived=%v name=%q", tg.IsArchived(), tg.Name())
	}
}

func TestTag_Archive_Unarchive_OnlyBumpOnChange(t *testing.T) {
	tg := newTag(t)
	tg.Unarchive(tt1) // no-op
	if !tg.UpdatedAt().Equal(tt0) {
		t.Fatal("no-op unarchive bumped updatedAt")
	}
	tg.Archive(tt1)
	if !tg.IsArchived() || !tg.UpdatedAt().Equal(tt1) {
		t.Fatalf("archive: %v / %v", tg.IsArchived(), tg.UpdatedAt())
	}
	tg.Archive(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)) // no-op
	if !tg.UpdatedAt().Equal(tt1) {
		t.Fatal("re-archive bumped updatedAt")
	}
	tg.Unarchive(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if tg.IsArchived() {
		t.Fatal("unarchive should clear the flag")
	}
}

func TestTag_UpdateName_OnlyBumpsOnChange(t *testing.T) {
	tg := newTag(t)
	tg.UpdateName("Travel", tt1)
	if !tg.UpdatedAt().Equal(tt0) {
		t.Fatal("same-name update bumped updatedAt")
	}
	tg.UpdateName("Holiday", tt1)
	if tg.Name() != "Holiday" || !tg.UpdatedAt().Equal(tt1) {
		t.Fatalf("rename: %q / %v", tg.Name(), tg.UpdatedAt())
	}
}

func TestTag_SetPosition(t *testing.T) {
	tg := newTag(t)
	tg.SetPosition(7)
	if tg.Position() != 7 {
		t.Fatalf("position=%d want 7", tg.Position())
	}
}
