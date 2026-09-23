// Budget cell comments: a flat thread per (element, month), shared with the
// budget. Reads are a windowed list; the budget reads are untouched.
package budget

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	commentMonthsDefault = 1
	commentMonthsMax     = 24
	// commentListCap bounds an unpaginated response. A budget with more
	// comments than this in one window is pathological; the cap is a guard, not
	// a paging design.
	commentListCap = 2000
)

func (s *Service) GetCommentList(ctx context.Context, userID vo.Id, req model.GetCommentListRequest) (*model.GetCommentListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	from, err := parseCommentFrom(req.From, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, err
	}
	months, err := parseCommentMonths(req.Months)
	if err != nil {
		return nil, err
	}

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canRead(b, userID) {
		return nil, accessDenied()
	}

	// One row past the cap is enough to know the window overflowed without
	// reading the rest of it.
	rows, err := s.comments.ListCommentsForWindow(ctx, budgetID, from, from.AddDate(0, months, 0), commentListCap+1)
	if err != nil {
		return nil, err
	}
	truncated := len(rows) > commentListCap
	if truncated {
		rows = rows[:commentListCap]
	}
	items := make([]model.CommentResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, commentResult(row))
	}
	return &model.GetCommentListResult{Items: items, Truncated: truncated}, nil
}

func commentResult(row model.BudgetCommentRow) model.CommentResult {
	return model.CommentResult{
		Id:        row.Comment.ID.String(),
		ElementId: row.ExternalID.String(),
		Period:    row.Comment.Period.Format(datetime.DateLayout),
		Comment:   row.Comment.Comment,
		Author:    model.UserResult{Id: row.Comment.UserID.String(), Avatar: row.AuthorAvatar, Name: row.AuthorName},
		CreatedAt: row.Comment.CreatedAt.Format(datetime.Layout),
		UpdatedAt: row.Comment.UpdatedAt.Format(datetime.Layout),
	}
}

// parseCommentFrom snaps the window start to the first of its month. An empty
// value means the caller's current month.
func parseCommentFrom(raw string, fallback time.Time) (time.Time, error) {
	if raw == "" {
		return model.FirstOfMonth(fallback), nil
	}
	t, err := time.Parse(datetime.DateLayout, raw)
	if err != nil {
		return time.Time{}, model.ValidateBlank(map[string]string{"from": ""})
	}
	return model.FirstOfMonth(t), nil
}

func parseCommentMonths(raw string) (int, error) {
	if raw == "" {
		return commentMonthsDefault, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > commentMonthsMax {
		return 0, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "months", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
		})
	}
	return n, nil
}

func (s *Service) CreateComment(ctx context.Context, userID vo.Id, req model.CreateCommentRequest) (*model.CreateCommentResult, error) {
	commentID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	externalID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"elementId": ""})
	}
	period, err := time.Parse(datetime.DateLayout, req.Period)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}
	period = model.FirstOfMonth(period)

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	// Guests may comment: a read-only role in the budget still has something to
	// say about the plan.
	if !s.canRead(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if period.Before(model.FirstOfMonth(b.budget.StartedAt)) {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}
	if end := b.endMonth(); end != nil && period.After(*end) {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}

	now := s.clock.Now()
	var stored *model.BudgetCommentRow
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// The comment id doubles as the idempotency key: a retried post finds
		// its own row and returns it rather than writing a second comment.
		already, cerr := s.ops.Claim(txCtx, commentID, now)
		if cerr != nil {
			return cerr
		}
		if already {
			// The operation guard's claimed-id table is shared by every feature and
			// keyed on the id alone, so a claim hit here only proves SOME create
			// landed under this id — not that it was this caller's, in this budget.
			// Re-check both before echoing the row back, or a caller who merely
			// knows another author's comment id (e.g. from a get-comment-list
			// response before their access was revoked) could retrieve it by
			// "retrying" a create with that id.
			row, gerr := s.comments.GetCommentRow(txCtx, commentID)
			if gerr != nil || !row.BudgetID.Equal(budgetID) || !row.Comment.UserID.Equal(userID) {
				return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
			}
			stored = row
			return nil
		}
		element, gerr := s.getElementSelfHeal(txCtx, budgetID, externalID, now)
		if gerr != nil {
			return gerr
		}
		c, verr := model.NewBudgetElementComment(commentID, element.ID, userID, req.Comment, period, now)
		if verr != nil {
			return verr
		}
		if ierr := s.comments.InsertComment(txCtx, c); ierr != nil {
			return ierr
		}
		row, gerr := s.comments.GetCommentRow(txCtx, commentID)
		if gerr != nil {
			return gerr
		}
		stored = row
		return s.ops.MarkHandled(txCtx, commentID, now)
	})
	if err != nil {
		return nil, err
	}
	// Logged from stored, not req: on the retry path both are now verified to
	// match the caller's own budget/comment, but req.BudgetId is still the raw,
	// unverified request value.
	reqctx.AddLogAttr(ctx, "budget_id", stored.BudgetID.String())
	reqctx.AddLogAttr(ctx, "comment_id", stored.Comment.ID.String())
	return &model.CreateCommentResult{Item: commentResult(*stored)}, nil
}

func (s *Service) UpdateComment(ctx context.Context, userID vo.Id, req model.UpdateCommentRequest) (*model.UpdateCommentResult, error) {
	commentID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, commentNotFound()
	}
	row, b, err := s.reachableComment(ctx, userID, commentID)
	if err != nil {
		return nil, err
	}
	if !row.Comment.UserID.Equal(userID) {
		return nil, commentForbidden()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}

	now := s.clock.Now()
	edited := row.Comment
	if verr := edited.Edit(req.Comment, now); verr != nil {
		return nil, verr
	}
	if err := s.comments.UpdateCommentText(ctx, &edited); err != nil {
		return nil, err
	}
	row.Comment = edited
	reqctx.AddLogAttr(ctx, "budget_id", row.BudgetID.String())
	reqctx.AddLogAttr(ctx, "comment_id", req.Id)
	return &model.UpdateCommentResult{Item: commentResult(*row)}, nil
}

func (s *Service) DeleteComment(ctx context.Context, userID vo.Id, req model.DeleteCommentRequest) (*model.DeleteCommentResult, error) {
	commentID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, commentNotFound()
	}
	row, b, err := s.reachableComment(ctx, userID, commentID)
	if err != nil {
		return nil, err
	}
	// The author cleans up after themselves; owner and admin moderate.
	if !row.Comment.UserID.Equal(userID) && !s.canDelete(b, userID) {
		return nil, commentForbidden()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if err := s.comments.DeleteComment(ctx, commentID); err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "budget_id", row.BudgetID.String())
	reqctx.AddLogAttr(ctx, "comment_id", req.Id)
	return &model.DeleteCommentResult{}, nil
}

// reachableComment loads a comment and its budget, answering "not found" for
// anything the caller cannot read — an id in someone else's budget must not be
// distinguishable from an id that never existed.
func (s *Service) reachableComment(ctx context.Context, userID, commentID vo.Id) (*model.BudgetCommentRow, *budgetAggregate, error) {
	row, err := s.comments.GetCommentRow(ctx, commentID)
	if err != nil {
		return nil, nil, commentNotFound()
	}
	b, err := s.loadAggregate(ctx, row.BudgetID)
	if err != nil {
		return nil, nil, commentNotFound()
	}
	if !s.canRead(b, userID) {
		return nil, nil, commentNotFound()
	}
	return row, b, nil
}

func commentNotFound() error {
	return &errs.ValidationError{Msg: "Comment not found", MsgCode: errs.CodeBudgetCommentNotFound}
}

func commentForbidden() error {
	return &errs.AccessDeniedError{Msg: "You can't change this comment", Code: errs.CodeBudgetCommentForbidden}
}
