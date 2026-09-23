package repo_test

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// commentFixture carries a budget with one element, the owner user, and a
// second element to exercise repoint. All ids belong to the shared budgetID
// budget seeded by saveBudget.
type commentFixture struct {
	budgetID       vo.Id
	elementID      vo.Id
	otherElementID vo.Id
	externalID     vo.Id
	userID         vo.Id
}

// newCommentFixture returns a budget with one element, the owner user already
// present, and a second element in the same budget (for repoint scenarios).
func newCommentFixture(t *testing.T) (context.Context, *budgetrepo.Repo, commentFixture) {
	t.Helper()
	ctx := context.Background()
	repo, _ := newRepo(t)
	saveBudget(t, repo, ctx)

	elementID := vo.NewId()
	externalID := vo.NewId()
	el := &model.BudgetElement{
		ID: elementID, BudgetID: vo.MustParseId(budgetID), ExternalID: externalID, Type: model.ElementCategory,
		SortKey: keyAt(0), CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.SaveElement(ctx, el); err != nil {
		t.Fatalf("SaveElement: %v", err)
	}

	otherElementID := vo.NewId()
	other := &model.BudgetElement{
		ID: otherElementID, BudgetID: vo.MustParseId(budgetID), ExternalID: vo.NewId(), Type: model.ElementCategory,
		SortKey: keyAt(1), CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	if err := repo.SaveElement(ctx, other); err != nil {
		t.Fatalf("SaveElement (other): %v", err)
	}

	return ctx, repo, commentFixture{
		budgetID: vo.MustParseId(budgetID), elementID: elementID, otherElementID: otherElementID,
		externalID: externalID, userID: vo.MustParseId(userA),
	}
}

func TestComments_InsertListUpdateDelete(t *testing.T) {
	ctx, r, fx := newCommentFixture(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC)

	c, err := model.NewBudgetElementComment(r.NextIdentity(), fx.elementID, fx.userID, "Trip to Lisbon", period, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatalf("InsertComment: %v", err)
	}

	rows, err := r.ListCommentsForWindow(ctx, fx.budgetID, period, period.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("ListCommentsForWindow: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	got := rows[0]
	if got.Comment.Comment != "Trip to Lisbon" {
		t.Fatalf("text=%q", got.Comment.Comment)
	}
	if !got.Comment.Period.Equal(period) {
		t.Fatalf("period=%v want %v", got.Comment.Period, period)
	}
	if !got.ExternalID.Equal(fx.externalID) {
		t.Fatalf("externalID=%v want %v", got.ExternalID, fx.externalID)
	}
	if !got.BudgetID.Equal(fx.budgetID) {
		t.Fatalf("budgetID=%v want %v", got.BudgetID, fx.budgetID)
	}
	if got.AuthorName == "" {
		t.Fatal("AuthorName empty: the users join is missing")
	}

	// A window that ends before the comment's month returns nothing.
	empty, err := r.ListCommentsForWindow(ctx, fx.budgetID, period.AddDate(0, -2, 0), period.AddDate(0, -1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("rows=%d want 0 outside the window", len(empty))
	}

	if err := c.Edit("Trip to Porto", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateCommentText(ctx, c); err != nil {
		t.Fatalf("UpdateCommentText: %v", err)
	}
	one, err := r.GetCommentRow(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCommentRow: %v", err)
	}
	if one.Comment.Comment != "Trip to Porto" {
		t.Fatalf("text=%q want updated", one.Comment.Comment)
	}

	if err := r.DeleteComment(ctx, c.ID); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := r.GetCommentRow(ctx, c.ID); err == nil {
		t.Fatal("GetCommentRow found a deleted comment")
	}
}

func TestComments_CascadeWithElement(t *testing.T) {
	ctx, r, fx := newCommentFixture(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := period

	c, err := model.NewBudgetElementComment(r.NextIdentity(), fx.elementID, fx.userID, "gone soon", period, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteElement(ctx, fx.elementID); err != nil {
		t.Fatalf("DeleteElement: %v", err)
	}
	if _, err := r.GetCommentRow(ctx, c.ID); err == nil {
		t.Fatal("comment survived its element")
	}
}

func TestComments_RepointAndDeleteByBudget(t *testing.T) {
	ctx, r, fx := newCommentFixture(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	c, err := model.NewBudgetElementComment(r.NextIdentity(), fx.elementID, fx.userID, "moves", period, period)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := r.RepointComments(ctx, fx.elementID, fx.otherElementID); err != nil {
		t.Fatalf("RepointComments: %v", err)
	}
	row, err := r.GetCommentRow(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.Comment.ElementID.Equal(fx.otherElementID) {
		t.Fatalf("elementID=%v want repointed", row.Comment.ElementID)
	}

	if err := r.DeleteCommentsByBudget(ctx, fx.budgetID); err != nil {
		t.Fatalf("DeleteCommentsByBudget: %v", err)
	}
	rows, err := r.ListCommentsForWindow(ctx, fx.budgetID, period, period.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%d want 0 after the budget-wide delete", len(rows))
	}
}
