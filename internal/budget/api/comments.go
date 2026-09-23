package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetCommentList handles GET /api/v1/budget/get-comment-list.
//
// @Summary  Comment threads for a budget's cells in a month window
// @Tags     Budget
// @Produce  json
// @Param    budgetId query string  true  "Budget id"
// @Param    from     query string  false "Window start (Y-m-d, snapped to first of month; defaults to the current month)"
// @Param    months   query integer false "Window length in months (1-24, default 1)"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetCommentListResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/get-comment-list [get]
func (h *Handlers) GetCommentList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.GetCommentListRequest{BudgetId: q.Get("budgetId"), From: q.Get("from"), Months: q.Get("months")}
	res, err := h.svc.GetCommentList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// CreateComment handles POST /api/v1/budget/create-comment.
//
// @Summary  Post a comment on a budget cell
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.CreateCommentRequest true "Create comment"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.CreateCommentResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  402 {object} apidoc.JsonResponseError
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/create-comment [post]
func (h *Handlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.CreateCommentRequest) (*model.CreateCommentResult, error) {
		reqctx.AddLogAttr(ctx, "element_id", req.ElementId)
		return h.svc.CreateComment(ctx, userID, req)
	})
}

// UpdateComment handles POST /api/v1/budget/update-comment.
//
// @Summary  Edit your own comment
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.UpdateCommentRequest true "Update comment"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.UpdateCommentResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  402 {object} apidoc.JsonResponseError
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/update-comment [post]
func (h *Handlers) UpdateComment(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateComment)
}

// DeleteComment handles POST /api/v1/budget/delete-comment.
//
// @Summary  Delete a comment (author, or the budget owner/admin)
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.DeleteCommentRequest true "Delete comment"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.DeleteCommentResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  402 {object} apidoc.JsonResponseError
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/delete-comment [post]
func (h *Handlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteComment)
}
