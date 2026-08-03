package model_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNewLabelSeedsIconAndTimestamps(t *testing.T) {
	now := time.Now()
	l := model.NewLabel(vo.NewId(), vo.NewId(), "Kid A", now)
	if l.Icon != "label" {
		t.Fatalf("icon = %q, want \"label\"", l.Icon)
	}
	if !l.CreatedAt.Equal(now) || !l.UpdatedAt.Equal(now) {
		t.Fatal("timestamps not seeded from now")
	}
	if l.IsArchived {
		t.Fatal("new label must not be archived")
	}
}

func TestLabelMutatorsBumpUpdatedAtOnlyOnChange(t *testing.T) {
	created := time.Now()
	later := created.Add(time.Hour)
	l := model.NewLabel(vo.NewId(), vo.NewId(), "Kid A", created)

	l.UpdateName("Kid A", later)
	if !l.UpdatedAt.Equal(created) {
		t.Fatal("no-op rename must not bump UpdatedAt")
	}
	l.UpdateName("Kid B", later)
	if !l.UpdatedAt.Equal(later) {
		t.Fatal("real rename must bump UpdatedAt")
	}

	l2 := model.NewLabel(vo.NewId(), vo.NewId(), "Pet", created)
	l2.Unarchive(later)
	if !l2.UpdatedAt.Equal(created) {
		t.Fatal("no-op unarchive must not bump UpdatedAt")
	}
	l2.Archive(later)
	if !l2.IsArchived || !l2.UpdatedAt.Equal(later) {
		t.Fatal("archive must set the flag and bump UpdatedAt")
	}
}
