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

	rows, err := s.comments.ListCommentsForWindow(ctx, budgetID, from, from.AddDate(0, months, 0))
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
