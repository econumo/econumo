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
	comments appbudget.CommentStore
	budgetID vo.Id
	elements map[string]vo.Id

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

	catFoodID := vo.MustParseId(f.BudgetElement(fixture.BudgetElement{
		BudgetID: budgetID.String(), ExternalID: vo.NewId().String(), Type: int(model.ElementCategory),
	}))

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
		ctx: context.Background(), svc: svc, comments: budgetRepo, budgetID: budgetID,
		elements: map[string]vo.Id{"cat-food": catFoodID},
		owner:    owner, guest: guest, member: member, pending: pending, stranger: stranger,
	}
}

// postComment seeds a comment directly through the CommentStore (not through
// CreateComment, which does not exist until Task 4).
func (h *commentHarness) postComment(t *testing.T, userID vo.Id, slug, period, text string) {
	t.Helper()
	elementID, ok := h.elements[slug]
	if !ok {
		t.Fatalf("unknown element slug %q", slug)
	}
	p, err := time.Parse(datetime.DateLayout, period)
	if err != nil {
		t.Fatalf("parse period %q: %v", period, err)
	}
	c, err := model.NewBudgetElementComment(h.comments.NextIdentity(), elementID, userID, text, p, p)
	if err != nil {
		t.Fatalf("NewBudgetElementComment: %v", err)
	}
	if err := h.comments.InsertComment(h.ctx, c); err != nil {
		t.Fatalf("InsertComment: %v", err)
	}
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
