package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestRules_RoundTrip(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	user := f.User(fixture.User{Email: "r@example.test"})
	cat := f.Category(fixture.Category{UserID: user})
	l1 := f.Label(fixture.Label{UserID: user, Name: "Trip"})
	l2 := f.Label(fixture.Label{UserID: user, Name: "Work"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	r := repo.NewRepo(db.Engine, db.TX)
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)

	catID, srcID := vo.MustParseId(cat), vo.MustParseId(src)
	rule := &model.ImportRule{
		ID: vo.NewId(), UserID: vo.MustParseId(user), SourceID: &srcID,
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee,
		MatchType: model.ImportRuleMatchTypeContains, MatchValue: "STARBUCKS", IsCaseSensitive: true,
		TargetCategoryID: &catID, LabelIDs: []vo.Id{vo.MustParseId(l1), vo.MustParseId(l2)}, Priority: 5,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := r.InsertRule(ctx, rule); err != nil {
		t.Fatalf("InsertRule: %v", err)
	}
	got, err := r.GetRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != srcID || !got.IsCaseSensitive || got.Priority != 5 || got.TargetCategoryID == nil || *got.TargetCategoryID != catID {
		t.Fatalf("round trip: %+v", got)
	}
	if len(got.LabelIDs) != 2 {
		t.Fatalf("labels: %+v", got.LabelIDs)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("created_at = %s", got.CreatedAt)
	}

	// update replaces the label set and clears the source scope
	rule.SourceID, rule.LabelIDs, rule.MatchValue, rule.IsCaseSensitive = nil, []vo.Id{vo.MustParseId(l2)}, "starbucks", false
	if err := r.UpdateRule(ctx, rule); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	got, _ = r.GetRule(ctx, rule.ID)
	if got.SourceID != nil || got.IsCaseSensitive || got.MatchValue != "starbucks" || len(got.LabelIDs) != 1 || got.LabelIDs[0].String() != l2 {
		t.Fatalf("after update: %+v", got)
	}

	// list is priority-ordered and carries labels
	second := &model.ImportRule{ID: vo.NewId(), UserID: vo.MustParseId(user), Action: model.ImportRuleActionSkip, MatchField: model.ImportRuleMatchFieldDescription, MatchType: model.ImportRuleMatchTypePrefix, MatchValue: "PAYMENT", Priority: 1, CreatedAt: now, UpdatedAt: now}
	if err := r.InsertRule(ctx, second); err != nil {
		t.Fatalf("InsertRule skip: %v", err)
	}
	list, err := r.ListRulesByUser(ctx, vo.MustParseId(user))
	if err != nil || len(list) != 2 || list[0].ID != second.ID || len(list[1].LabelIDs) != 1 {
		t.Fatalf("ListRulesByUser: %v %+v", err, list)
	}

	if err := r.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if _, err := r.GetRule(ctx, rule.ID); err == nil {
		t.Fatal("deleted rule must be gone")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
	// the join rows went with it (FK cascade)
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(`SELECT count(*) FROM import_rule_labels WHERE rule_id = ?`), rule.ID.String()).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rule labels left behind: %d %v", n, err)
	}
}

func TestLinkAppliedLabels_ReplaceAndList(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	user := f.User(fixture.User{Email: "r@example.test"})
	l1 := f.Label(fixture.Label{UserID: user, Name: "A"})
	l2 := f.Label(fixture.Label{UserID: user, Name: "B"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	link := f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: src, ExternalAccountID: "card", ExternalTransactionID: "t1", Status: model.ImportLinkStatusQueued, ExternalAmount: "1", ExternalPostedAt: time.Now()})
	r := repo.NewRepo(db.Engine, db.TX)
	linkID := vo.MustParseId(link)
	if err := r.ReplaceLinkAppliedLabels(ctx, linkID, []vo.Id{vo.MustParseId(l1), vo.MustParseId(l2)}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, err := r.ListLinkAppliedLabels(ctx, linkID)
	if err != nil || len(got) != 2 {
		t.Fatalf("list: %v %v", got, err)
	}
	if err := r.ReplaceLinkAppliedLabels(ctx, linkID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ = r.ListLinkAppliedLabels(ctx, linkID); len(got) != 0 {
		t.Fatalf("after clear: %v", got)
	}
	all, err := r.ListLinksByUser(ctx, vo.MustParseId(user))
	if err != nil || len(all) != 1 || all[0].ID != linkID {
		t.Fatalf("ListLinksByUser: %v %+v", err, all)
	}
}
