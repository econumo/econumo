package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func commentIDs(t *testing.T) (vo.Id, vo.Id, vo.Id) {
	t.Helper()
	id, err := vo.ParseId("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	elementID, err := vo.ParseId("22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := vo.ParseId("33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatal(err)
	}
	return id, elementID, userID
}

func TestNewBudgetElementComment_TrimsAndSnapsPeriod(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	now := time.Date(2026, 5, 17, 10, 30, 0, 0, time.UTC)
	period := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)

	c, err := model.NewBudgetElementComment(id, elementID, userID, "  Trip to Lisbon  ", period, now)
	if err != nil {
		t.Fatalf("NewBudgetElementComment: %v", err)
	}
	if c.Comment != "Trip to Lisbon" {
		t.Fatalf("Comment=%q want trimmed", c.Comment)
	}
	if !c.Period.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Period=%v want first of month", c.Period)
	}
	if !c.CreatedAt.Equal(now) || !c.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps=%v/%v want %v", c.CreatedAt, c.UpdatedAt, now)
	}
}

func TestNewBudgetElementComment_Blank(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	now := time.Now().UTC()

	_, err := model.NewBudgetElementComment(id, elementID, userID, "   \t\n ", now, now)
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err=%v want ValidationError", err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Key != "comment" || ve.Fields[0].Code != errs.CodeIsBlank {
		t.Fatalf("fields=%+v want one comment/is_blank", ve.Fields)
	}
}

// Review Focus 1: the limit is 500 RUNES, not bytes. A 500-emoji comment is
// 2000 bytes and must be accepted.
func TestNewBudgetElementComment_LengthBoundaryIsRunes(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	now := time.Now().UTC()

	ok500 := strings.Repeat("\U0001F600", 500)
	if _, err := model.NewBudgetElementComment(id, elementID, userID, ok500, now, now); err != nil {
		t.Fatalf("500 runes rejected: %v", err)
	}

	tooLong := strings.Repeat("\U0001F600", 501)
	_, err := model.NewBudgetElementComment(id, elementID, userID, tooLong, now, now)
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err=%v want ValidationError", err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Code != errs.CodeTooLong {
		t.Fatalf("fields=%+v want comment/too_long", ve.Fields)
	}
}

func TestBudgetElementComment_Edit(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	created := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	edited := created.Add(time.Hour)

	c, err := model.NewBudgetElementComment(id, elementID, userID, "first", created, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Edit("  first  ", edited); err != nil {
		t.Fatal(err)
	}
	if !c.UpdatedAt.Equal(created) {
		t.Fatalf("UpdatedAt moved on a no-op edit: %v", c.UpdatedAt)
	}
	if err := c.Edit("second", edited); err != nil {
		t.Fatal(err)
	}
	if c.Comment != "second" || !c.UpdatedAt.Equal(edited) {
		t.Fatalf("Comment=%q UpdatedAt=%v want second/%v", c.Comment, c.UpdatedAt, edited)
	}
	if err := c.Edit("", edited); err == nil {
		t.Fatal("blank edit accepted")
	}
	if c.Comment != "second" {
		t.Fatalf("Comment=%q mutated by a rejected edit", c.Comment)
	}
}
