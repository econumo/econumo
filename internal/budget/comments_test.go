package budget_test

import (
	"context"
	"testing"
	"time"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/model"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// commentHarness wires a real budget.Service over a migrated sqlite database,
// the same way internal/budget/api/harness_test.go does, minus the HTTP layer.
// The budget aggregate (budget row, one "cat-food" element, and the
// owner/guest/member/pending/stranger access rows) is seeded directly through
// the fixture builder rather than through CreateBudget/GrantAccess/AcceptAccess,
// since those use cases pull in unrelated machinery (owned accounts, category
// seeding, connections) this suite has no need to exercise.
type commentHarness struct {
	ctx      context.Context
	svc      *appbudget.Service
	f        *fixture.Builder
	budgetID vo.Id
	// catFood is the "cat-food" element's EXTERNAL id (what the wire and
	// CreateComment's ElementId call it), not the internal budgets_elements.id.
	catFood string

	owner, guest, member, pending, stranger vo.Id
}

// newCommentHarness seeds a budget started 2026-04-01, owned by owner, with one
// category element ("cat-food") and four other users: guest (accepted,
// read-only), member (accepted, full edit), pending (invited but not
// accepted) and stranger (no access row at all).
func newCommentHarness(t *testing.T) *commentHarness {
	t.Helper()
	tdb := dbtest.NewSQLite(t)
	txm := tdb.TX
	f := fixture.New(t, tdb)

	// Explicit emails: the fixture's default email derives from the id's first
	// 8 hex characters, which is the slow-moving high part of a UUIDv7
	// timestamp and collides across ids minted within the same ~65s window —
	// exactly what five back-to-back calls here would do.
	owner := vo.MustParseId(f.User(fixture.User{Name: "Owner", Email: "owner@comments.test"}))
	guest := vo.MustParseId(f.User(fixture.User{Name: "Guest", Email: "guest@comments.test"}))
	member := vo.MustParseId(f.User(fixture.User{Name: "Member", Email: "member@comments.test"}))
	pending := vo.MustParseId(f.User(fixture.User{Name: "Pending", Email: "pending@comments.test"}))
	stranger := vo.MustParseId(f.User(fixture.User{Name: "Stranger", Email: "stranger@comments.test"}))

	started := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	budgetID := vo.MustParseId(f.Budget(fixture.Budget{UserID: owner.String(), StartedAt: &started}))

	catFood := vo.NewId().String()
	f.BudgetElement(fixture.BudgetElement{
		BudgetID: budgetID.String(), ExternalID: catFood, Type: int(model.ElementCategory),
	})

	f.BudgetAccess(budgetID.String(), guest.String(), int(model.BudgetRoleGuest), true)
	f.BudgetAccess(budgetID.String(), member.String(), int(model.BudgetRoleUser), true)
	f.BudgetAccess(budgetID.String(), pending.String(), int(model.BudgetRoleUser), false)

	userRepo := userrepo.NewRepo("sqlite", txm)
	accountRepo := accountrepo.NewRepo("sqlite", txm)
	categoryRepo := categoryrepo.NewRepo("sqlite", txm)
	tagRepo := tagrepo.NewRepo("sqlite", txm)
	payeeRepo := payeerepo.NewRepo("sqlite", txm)
	currencyLookup := currencyrepo.New("sqlite", txm)

	budgetRepo := budgetrepo.NewRepo("sqlite", txm)
	budgetReadRepo := budgetrepo.NewReadRepo("sqlite", txm)
	rateProvider := currencyrepo.NewRateProvider("sqlite", txm, currencyLookup, fixture.USD)
	convertor := domcurrency.NewConvertor(rateProvider)
	clk := clock.New()
	svc := appbudget.NewService(
		budgetRepo, budgetReadRepo, convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountRepo),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagRepo), server.NewBudgetPayeeMetadataLookup(payeeRepo)),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)),
		operationrepo.NewGuard("sqlite", txm),
		txm, clk,
	)

	return &commentHarness{
		ctx: context.Background(), svc: svc, f: f, budgetID: budgetID, catFood: catFood,
		owner: owner, guest: guest, member: member, pending: pending, stranger: stranger,
	}
}

// newID mints a fresh id, used both as a comment's own id and, for a slug
// that names no seeded element (e.g. "uncategorized"), as a stand-in external
// element id that resolves to nothing.
func (h *commentHarness) newID() vo.Id { return vo.NewId() }

// elementExternalID resolves a test slug to the EXTERNAL element id
// CreateCommentRequest.ElementId expects. Only "cat-food" is seeded; any other
// slug yields a fresh id that matches no budget element.
func (h *commentHarness) elementExternalID(slug string) string {
	if slug == "cat-food" {
		return h.catFood
	}
	return vo.NewId().String()
}

// create posts a comment through the CreateComment use case.
func (h *commentHarness) create(userID, id vo.Id, slug, period, text string) (*model.CreateCommentResult, error) {
	return h.svc.CreateComment(h.ctx, userID, model.CreateCommentRequest{
		Id: id.String(), BudgetId: h.budgetID.String(), ElementId: h.elementExternalID(slug), Period: period, Comment: text,
	})
}

// mustCreate posts a comment (with a fresh id) and fails the test on error.
func (h *commentHarness) mustCreate(t *testing.T, userID vo.Id, slug, period, text string) *model.CreateCommentResult {
	t.Helper()
	res, err := h.create(userID, h.newID(), slug, period, text)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	return res
}

// postComment seeds a comment through CreateComment (the use case Task 4
// adds); Task 3 seeded through CommentStore.InsertComment directly only
// because CreateComment did not exist yet.
func (h *commentHarness) postComment(t *testing.T, userID vo.Id, slug, period, text string) {
	t.Helper()
	if _, err := h.create(userID, h.newID(), slug, period, text); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
}

// update edits a comment through the UpdateComment use case.
func (h *commentHarness) update(userID vo.Id, id, text string) (*model.UpdateCommentResult, error) {
	return h.svc.UpdateComment(h.ctx, userID, model.UpdateCommentRequest{Id: id, Comment: text})
}

// remove deletes a comment through the DeleteComment use case.
func (h *commentHarness) remove(userID vo.Id, id string) (*model.DeleteCommentResult, error) {
	return h.svc.DeleteComment(h.ctx, userID, model.DeleteCommentRequest{Id: id})
}

// setEndMonth sets the budget's end month as its owner, through UpdateBudget
// (there is no dedicated set-end-month use case).
func (h *commentHarness) setEndMonth(t *testing.T, period string) {
	t.Helper()
	if _, err := h.svc.UpdateBudget(h.ctx, h.owner, model.UpdateBudgetRequest{
		Id: h.budgetID.String(), Name: "Budget", CurrencyId: fixture.USD, EndDate: &period,
	}); err != nil {
		t.Fatalf("UpdateBudget (setEndMonth): %v", err)
	}
}

// commentInAnotherBudget seeds a second budget - a different owner, a
// different element - with one comment, and returns the comment's id: a
// target this harness's own users have no access to at all.
func (h *commentHarness) commentInAnotherBudget(t *testing.T) string {
	t.Helper()
	foreignOwner := vo.MustParseId(h.f.User(fixture.User{Name: "Foreign Owner", Email: "foreign-owner@comments.test"}))
	foreignStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	foreignBudgetID := vo.MustParseId(h.f.Budget(fixture.Budget{UserID: foreignOwner.String(), StartedAt: &foreignStart}))
	externalID := vo.NewId().String()
	h.f.BudgetElement(fixture.BudgetElement{
		BudgetID: foreignBudgetID.String(), ExternalID: externalID, Type: int(model.ElementCategory),
	})

	res, err := h.svc.CreateComment(h.ctx, foreignOwner, model.CreateCommentRequest{
		Id: h.newID().String(), BudgetId: foreignBudgetID.String(), ElementId: externalID,
		Period: "2026-05-01", Comment: "foreign budget comment",
	})
	if err != nil {
		t.Fatalf("CreateComment (foreign budget): %v", err)
	}
	return res.Item.Id
}

// list calls GetCommentList and fails the test on error.
func (h *commentHarness) list(t *testing.T, userID vo.Id, from, months string) *model.GetCommentListResult {
	t.Helper()
	res, err := h.rawList(userID, from, months)
	if err != nil {
		t.Fatalf("GetCommentList: %v", err)
	}
	return res
}

// rawList calls GetCommentList and returns the error for the caller to assert.
func (h *commentHarness) rawList(userID vo.Id, from, months string) (*model.GetCommentListResult, error) {
	return h.svc.GetCommentList(h.ctx, userID, model.GetCommentListRequest{
		BudgetId: h.budgetID.String(), From: from, Months: months,
	})
}

// archive archives the budget as its owner.
func (h *commentHarness) archive(t *testing.T) {
	t.Helper()
	if _, err := h.svc.ArchiveBudget(h.ctx, h.owner, model.ArchiveBudgetRequest{Id: h.budgetID.String()}); err != nil {
		t.Fatalf("ArchiveBudget: %v", err)
	}
}

// revoke revokes userID's access, performed by the owner (the only role
// canShare allows).
func (h *commentHarness) revoke(t *testing.T, userID vo.Id) {
	t.Helper()
	if _, err := h.svc.RevokeAccess(h.ctx, h.owner, model.RevokeAccessRequest{
		BudgetId: h.budgetID.String(), UserId: userID.String(),
	}); err != nil {
		t.Fatalf("RevokeAccess: %v", err)
	}
}

func TestGetCommentList_WindowAndOrder(t *testing.T) {
	h := newCommentHarness(t) // budget started 2026-04-01, owner + guest, element CatFood

	h.postComment(t, h.owner, "cat-food", "2026-05-01", "May note")
	h.postComment(t, h.owner, "cat-food", "2026-06-01", "June note")
	// Carried finding from Task 2's review: the window is half-open, so a
	// comment dated exactly at the upper boundary (from + months) must be
	// excluded, not just one dated past it.
	h.postComment(t, h.owner, "cat-food", "2026-07-01", "July note (upper boundary, excluded)")

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 1 || res.Items[0].Comment != "May note" {
		t.Fatalf("items=%+v want only the May note", res.Items)
	}
	if res.Items[0].Period != "2026-05-01" {
		t.Fatalf("period=%q want Y-m-d", res.Items[0].Period)
	}
	if res.Truncated {
		t.Fatal("Truncated set on a two-row window")
	}

	res = h.list(t, h.owner, "2026-05-01", "2")
	if len(res.Items) != 2 {
		t.Fatalf("items=%d want 2 over a two-month window (July sits at the exclusive upper boundary)", len(res.Items))
	}
	if res.Items[0].Comment != "May note" || res.Items[1].Comment != "June note" {
		t.Fatalf("order=%+v want chronological", res.Items)
	}
}

// Review Focus 3: malformed window parameters must never widen the window.
func TestGetCommentList_BadWindowParams(t *testing.T) {
	h := newCommentHarness(t)

	for _, months := range []string{"0", "25", "abc", "-1"} {
		if _, err := h.rawList(h.owner, "2026-05-01", months); err == nil {
			t.Fatalf("months=%q accepted", months)
		}
	}
	if _, err := h.rawList(h.owner, "not-a-date", "1"); err == nil {
		t.Fatal("from=not-a-date accepted")
	}
	// A mid-month "from" snaps to the first of that month rather than erroring.
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "May note")
	res := h.list(t, h.owner, "2026-05-23", "1")
	if len(res.Items) != 1 {
		t.Fatalf("items=%d want the May note after snapping", len(res.Items))
	}
}

func TestGetCommentList_GuestReadsPendingDenied(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "shared")

	res := h.list(t, h.guest, "2026-05-01", "1")
	if len(res.Items) != 1 {
		t.Fatalf("guest saw %d items, want 1", len(res.Items))
	}
	if _, err := h.rawList(h.pending, "2026-05-01", "1"); err == nil {
		t.Fatal("a pending invitee read the thread")
	}
	if _, err := h.rawList(h.stranger, "2026-05-01", "1"); err == nil {
		t.Fatal("a stranger read the thread")
	}
}

// Review Focus 5: a revoked member's comments keep rendering their author.
func TestGetCommentList_RevokedAuthorStillRenders(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.member, "cat-food", "2026-05-01", "written before the revoke")
	h.revoke(t, h.member)

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 1 {
		t.Fatalf("items=%d want the revoked member's comment", len(res.Items))
	}
	if res.Items[0].Author.Name == "" || res.Items[0].Author.Id == "" {
		t.Fatalf("author=%+v want the stored user's identity", res.Items[0].Author)
	}
}

func TestGetCommentList_ArchivedBudgetStillReadable(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "before archiving")
	h.archive(t)

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 1 {
		t.Fatalf("items=%d want the thread on an archived budget", len(res.Items))
	}
}

// Carried finding from Task 3's review: nothing exercised the read defaults.
func TestGetCommentList_DefaultsMonthsAndFrom(t *testing.T) {
	h := newCommentHarness(t)
	currentMonth := model.FirstOfMonth(time.Now().UTC()).Format(datetime.DateLayout)
	h.postComment(t, h.owner, "cat-food", currentMonth, "this month, no window params")

	res := h.list(t, h.owner, "", "")
	if len(res.Items) != 1 || res.Items[0].Period != currentMonth {
		t.Fatalf("items=%+v want the caller's current month via an empty from/months", res.Items)
	}
}

func TestGetCommentList_EmptyMonthsDefaultsToOne(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "May note")
	h.postComment(t, h.owner, "cat-food", "2026-06-01", "June note (next month, excluded)")

	res := h.list(t, h.owner, "2026-05-01", "")
	if len(res.Items) != 1 || res.Items[0].Comment != "May note" {
		t.Fatalf("items=%+v want only May via an empty months (defaults to 1)", res.Items)
	}
}

func TestCreateComment_GuestMayPost(t *testing.T) {
	h := newCommentHarness(t)

	res, err := h.create(h.guest, h.newID(), "cat-food", "2026-05-01", "  Trip to Lisbon  ")
	if err != nil {
		t.Fatalf("guest post: %v", err)
	}
	if res.Item.Comment != "Trip to Lisbon" {
		t.Fatalf("comment=%q want trimmed", res.Item.Comment)
	}
	if res.Item.Author.Id != h.guest.String() {
		t.Fatalf("author=%q want the guest", res.Item.Author.Id)
	}
	if res.Item.ElementId != h.catFood {
		t.Fatalf("elementId=%q want the external id", res.Item.ElementId)
	}
}

// Review Focus 4: the same client id posted twice leaves one row.
func TestCreateComment_IdempotentOnClientId(t *testing.T) {
	h := newCommentHarness(t)
	id := h.newID()

	first, err := h.create(h.owner, id, "cat-food", "2026-05-01", "double tap")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.create(h.owner, id, "cat-food", "2026-05-01", "double tap")
	if err != nil {
		t.Fatalf("retry rejected: %v", err)
	}
	if first.Item.Id != second.Item.Id {
		t.Fatalf("ids differ: %q vs %q", first.Item.Id, second.Item.Id)
	}
	list := h.list(t, h.owner, "2026-05-01", "1")
	if len(list.Items) != 1 {
		t.Fatalf("items=%d want exactly one row", len(list.Items))
	}
}

func TestCreateComment_PeriodBounds(t *testing.T) {
	h := newCommentHarness(t) // started 2026-04-01

	if _, err := h.create(h.owner, h.newID(), "cat-food", "2026-03-01", "too early"); err == nil {
		t.Fatal("a period before the budget start was accepted")
	}
	h.setEndMonth(t, "2026-07-01")
	if _, err := h.create(h.owner, h.newID(), "cat-food", "2026-08-01", "too late"); err == nil {
		t.Fatal("a period past the end month was accepted")
	}
}

func TestCreateComment_ArchivedAndUncategorized(t *testing.T) {
	h := newCommentHarness(t)

	if _, err := h.create(h.owner, h.newID(), "uncategorized", "2026-05-01", "nope"); err == nil {
		t.Fatal("uncategorized accepted a comment")
	}
	h.archive(t)
	_, err := h.create(h.owner, h.newID(), "cat-food", "2026-05-01", "nope")
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetArchived {
		t.Fatalf("err=%v want budget.archived", err)
	}
}

func TestUpdateComment_AuthorOnly(t *testing.T) {
	h := newCommentHarness(t)
	own := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "mine")

	if _, err := h.update(h.member, own.Item.Id, "mine, edited"); err != nil {
		t.Fatalf("author edit rejected: %v", err)
	}
	_, err := h.update(h.owner, own.Item.Id, "not yours")
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetCommentForbidden {
		t.Fatalf("err=%v want comment_forbidden for the budget owner", err)
	}
}

func TestDeleteComment_AuthorOrModerator(t *testing.T) {
	h := newCommentHarness(t)

	mine := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "mine")
	if _, err := h.remove(h.member, mine.Item.Id); err != nil {
		t.Fatalf("author delete rejected: %v", err)
	}

	theirs := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "moderated")
	if _, err := h.remove(h.owner, theirs.Item.Id); err != nil {
		t.Fatalf("owner moderation rejected: %v", err)
	}

	guests := h.mustCreate(t, h.guest, "cat-food", "2026-05-01", "guest note")
	_, err := h.remove(h.member, guests.Item.Id)
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetCommentForbidden {
		t.Fatalf("err=%v want comment_forbidden for a non-author non-admin", err)
	}
}

// Review Focus 2: a comment id from another budget must look absent, not
// forbidden — existence is not disclosed to an outsider.
func TestUpdateDeleteComment_ForeignBudgetLooksAbsent(t *testing.T) {
	h := newCommentHarness(t)
	foreign := h.commentInAnotherBudget(t)

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"update", func() error { _, err := h.update(h.owner, foreign, "peek"); return err }},
		{"delete", func() error { _, err := h.remove(h.owner, foreign); return err }},
		{"unknown-id", func() error { _, err := h.update(h.owner, h.newID().String(), "peek"); return err }},
	} {
		ve, ok := errs.AsValidation(tc.run())
		if !ok || ve.MsgCode != errs.CodeBudgetCommentNotFound {
			t.Fatalf("%s: want a coded comment_not_found validation error, got %+v", tc.name, ve)
		}
	}
}
