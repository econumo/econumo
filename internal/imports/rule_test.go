package imports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func classifyReq(h *harness, cat string) model.CreateImportRuleRequest {
	return model.CreateImportRuleRequest{Id: vo.NewId().String(), ImportRuleSpec: model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee, MatchType: model.ImportRuleMatchTypeContains,
		MatchValue: "Blue Bottle", CategoryId: &cat, Priority: 5,
	}}
}

func hasField(ve *errs.ValidationError, key string) bool {
	for _, f := range ve.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

func TestRules_CreateListUpdateDelete(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	label := h.f.Label(fixture.Label{UserID: userA, Name: "Work"})
	h.entities.add(h.entities.categories, uid, cat, "Coffee")
	h.entities.add(h.entities.labels, uid, label, "Work")

	req := classifyReq(h, cat)
	req.LabelIds = []string{label}
	created, err := h.svc.CreateRule(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if created.Id != req.Id || created.CategoryId != cat || len(created.LabelIds) != 1 || created.SourceId != "" {
		t.Fatalf("created: %+v", created)
	}

	list, err := h.svc.GetRuleList(context.Background(), uid)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}

	upd := model.UpdateImportRuleRequest{Id: req.Id, ImportRuleSpec: req.ImportRuleSpec}
	upd.MatchValue = "STARBUCKS COFFEE"
	upd.LabelIds = nil
	updated, err := h.svc.UpdateRule(context.Background(), uid, upd)
	if err != nil || updated.MatchValue != "STARBUCKS COFFEE" || len(updated.LabelIds) != 0 {
		t.Fatalf("update: %v %+v", err, updated)
	}

	if err := h.svc.DeleteRule(context.Background(), uid, model.DeleteImportRuleRequest{Id: req.Id}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = h.svc.GetRuleList(context.Background(), uid)
	if len(list.Items) != 0 {
		t.Fatalf("rule must be gone: %+v", list.Items)
	}
}

func TestRules_RejectsForeignTargets(t *testing.T) {
	h := setup(t)
	uid := vo.MustParseId(userA)
	foreignCat := h.f.Category(fixture.Category{UserID: userB, Name: "Theirs"})
	h.entities.add(h.entities.categories, vo.MustParseId(userB), foreignCat, "Theirs")
	_, err := h.svc.CreateRule(context.Background(), uid, classifyReq(h, foreignCat))
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || !hasField(ve, "categoryId") {
		t.Fatalf("a category the caller does not own must be a categoryId validation error, got %v", err)
	}
	other := h.f.ImportSource(fixture.ImportSource{UserID: userB, Provider: model.ImportProviderSimpleFIN, Name: "Theirs"})
	req := classifyReq(h, "")
	req.CategoryId = nil
	req.TagId = nil
	req.SourceId = &other
	req.Action = model.ImportRuleActionSkip
	if _, err := h.svc.CreateRule(context.Background(), uid, req); !isNotFound(err) {
		t.Fatalf("another user's source must be not-found, got %v", err)
	}
}

func TestRules_OtherUsersRuleIsNotFound(t *testing.T) {
	h := setup(t)
	id := h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "x"})
	err := h.svc.DeleteRule(context.Background(), vo.MustParseId(userA), model.DeleteImportRuleRequest{Id: id})
	if !isNotFound(err) {
		t.Fatalf("got %v", err)
	}
	list, _ := h.svc.GetRuleList(context.Background(), vo.MustParseId(userA))
	if len(list.Items) != 0 {
		t.Fatal("must not list another user's rule")
	}
}
