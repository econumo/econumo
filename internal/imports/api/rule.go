package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// GetRuleList handles GET /api/v1/import/get-rule-list (auth).
//
// @Summary     List import rules
// @Description Returns the caller's classify/skip rules, newest first.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportRuleListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-rule-list [get]
func (h *Handlers) GetRuleList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetRuleList)
}

// CreateRule handles POST /api/v1/import/create-rule (auth).
//
// @Summary     Create an import rule
// @Description Creates a classify or skip rule for matching future/existing imports.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateImportRuleRequest true "Create rule request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ImportRuleResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/create-rule [post]
func (h *Handlers) CreateRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.CreateRule)
}

// UpdateRule handles POST /api/v1/import/update-rule (auth).
//
// @Summary     Update an import rule
// @Description Replaces a rule's match/target spec. Requires ownership.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateImportRuleRequest true "Update rule request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ImportRuleResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/update-rule [post]
func (h *Handlers) UpdateRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateRule)
}

// DeleteRule handles POST /api/v1/import/delete-rule (auth).
//
// @Summary     Delete an import rule
// @Description Deletes a rule. Requires ownership.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.DeleteImportRuleRequest true "Delete rule request"
// @Success     200     {object} apidoc.JsonResponseOk
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/delete-rule [post]
func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.DeleteImportRuleRequest) (struct{}, error) {
		return struct{}{}, h.svc.DeleteRule(ctx, userID, req)
	})
}

// PreviewRule handles POST /api/v1/import/preview-rule (auth).
//
// PreviewRule is a POST *read*, like list-external-accounts: the whole rule
// spec plus its scope travels in the body.
//
// @Summary     Count the existing imports a rule would touch
// @Description Reports how many imported transactions in scope match the rule spec, and how many of those were already hand-edited.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.PreviewImportRuleRequest true "Rule spec + scope"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.PreviewImportRuleResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/preview-rule [post]
func (h *Handlers) PreviewRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.PreviewRule)
}

// ApplyRule handles POST /api/v1/import/apply-rule (auth).
//
// @Summary     Apply a saved rule to existing imports
// @Description Re-classifies (or skips) every already-imported transaction in scope that matches the rule, skipping hand-edited ones unless includeEdited is set.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ApplyImportRuleRequest true "Rule id + scope"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ApplyImportRuleResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/apply-rule [post]
func (h *Handlers) ApplyRule(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ApplyRule)
}

// SuggestRules handles POST /api/v1/import/suggest-rules (auth).
//
// @Summary     Ask the configured AI for rule suggestions
// @Description Returns AI-proposed rule specs for imports in scope. 400 import.ai_disabled when no AI provider is configured on this server.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.SuggestImportRulesRequest true "Scope"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SuggestImportRulesResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/suggest-rules [post]
func (h *Handlers) SuggestRules(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SuggestRules)
}
