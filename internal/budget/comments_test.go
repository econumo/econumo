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
	"github.com/econumo/econumo/internal/shared/port"
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
	tdb      *dbtest.DB
	budgetID vo.Id
	// catFood and catTransport are two elements' EXTERNAL ids (what the wire and
	// CreateComment's ElementId call it), not the internal budgets_elements.id -
	// the second element exists so a merge test has a real target to repoint onto.
	catFood, catTransport string

	// budgetRepo and clk back mergeCategory, which drives a real MergeService
	// over the same database the harness's own Service reads and writes.
	budgetRepo *budgetrepo.Repo
	clk        port.Clock

	owner, admin, guest, member, pending, stranger vo.Id
}

// newCommentHarness seeds a budget started 2026-04-01, owned by owner, with one
// category element ("cat-food") and five other users: admin (accepted,
// owner-equivalent moderation), guest (accepted, read-only), member (accepted,
// full edit), pending (invited but not accepted) and stranger (no access row
// at all).
func newCommentHarness(t *testing.T) *commentHarness {
	t.Helper()
	tdb := dbtest.NewSQLite(t)
	txm := tdb.TX
	f := fixture.New(t, tdb)

	// Explicit emails: the fixture's default email derives from the id's first
	// 8 hex characters, which is the slow-moving high part of a UUIDv7
	// timestamp and collides across ids minted within the same ~65s window —
	// exactly what six back-to-back calls here would do.
	owner := vo.MustParseId(f.User(fixture.User{Name: "Owner", Email: "owner@comments.test"}))
	admin := vo.MustParseId(f.User(fixture.User{Name: "Admin", Email: "admin@comments.test"}))
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
	catTransport := vo.NewId().String()
	f.BudgetElement(fixture.BudgetElement{
		BudgetID: budgetID.String(), ExternalID: catTransport, Type: int(model.ElementCategory),
	})

	f.BudgetAccess(budgetID.String(), admin.String(), int(model.BudgetRoleAdmin), true)
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
		ctx: context.Background(), svc: svc, f: f, tdb: tdb, budgetID: budgetID, catFood: catFood, catTransport: catTransport,
		budgetRepo: budgetRepo, clk: clk,
		owner: owner, admin: admin, guest: guest, member: member, pending: pending, stranger: stranger,
	}
}

// newID mints a fresh id, used as a comment's own client-supplied id.
func (h *commentHarness) newID() vo.Id { return vo.NewId() }

// unknownElement returns a fresh, well-formed external id matching no seeded
// budget element — for a test that explicitly wants "an id naming nothing",
// as opposed to mistyping a slug in elementExternalID.
func (h *commentHarness) unknownElement() string { return vo.NewId().String() }

// elementExternalID resolves a test slug to the EXTERNAL element id
// CreateCommentRequest.ElementId expects. "cat-food" is the one seeded
// element; model.UncategorizedID ("uncategorized") is the presentation-only
// pseudo-element's own literal wire value (deliberately not a UUID, per its
// doc comment), so using it here exercises the real elementId-parse refusal
// rather than a random id that merely resolves to "element not found". Any
// other slug is almost certainly a typo, so it fails loudly rather than
// silently becoming a fresh unknown-element id.
func (h *commentHarness) elementExternalID(t *testing.T, slug string) string {
	t.Helper()
	switch slug {
	case "cat-food":
		return h.catFood
	case "cat-transport":
		return h.catTransport
	case model.UncategorizedID:
		return model.UncategorizedID
	default:
		t.Fatalf("unknown element slug %q (use h.unknownElement() for an id naming nothing)", slug)
		return ""
	}
}

// create posts a comment through the CreateComment use case.
func (h *commentHarness) create(t *testing.T, userID, id vo.Id, slug, period, text string) (*model.CreateCommentResult, error) {
	t.Helper()
	return h.svc.CreateComment(h.ctx, userID, model.CreateCommentRequest{
		Id: id.String(), BudgetId: h.budgetID.String(), ElementId: h.elementExternalID(t, slug), Period: period, Comment: text,
	})
}

// mustCreate posts a comment (with a fresh id) and fails the test on error.
func (h *commentHarness) mustCreate(t *testing.T, userID vo.Id, slug, period, text string) *model.CreateCommentResult {
	t.Helper()
	res, err := h.create(t, userID, h.newID(), slug, period, text)
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
	if _, err := h.create(t, userID, h.newID(), slug, period, text); err != nil {
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

// reset resets the budget's start month as its owner. startedAt is a bare
// Y-m-d date (the brief's test cases pass "2026-05-01"); ResetBudgetRequest
// wants the full datetime.Layout, so it is expanded to midnight here.
func (h *commentHarness) reset(t *testing.T, startedAt string) {
	t.Helper()
	if _, err := h.svc.ResetBudget(h.ctx, h.owner, model.ResetBudgetRequest{
		Id: h.budgetID.String(), StartedAt: startedAt + " 00:00:00",
	}); err != nil {
		t.Fatalf("ResetBudget: %v", err)
	}
}

// clone clones h.budgetID as its owner and returns the copy's id.
func (h *commentHarness) clone(t *testing.T, name, startDate string, withLimits bool) vo.Id {
	t.Helper()
	res, err := h.svc.CloneBudget(h.ctx, h.owner, model.CloneBudgetRequest{
		Id: h.budgetID.String(), NewId: h.newID().String(), Name: name, StartDate: startDate, WithLimits: withLimits,
	})
	if err != nil {
		t.Fatalf("CloneBudget: %v", err)
	}
	return vo.MustParseId(res.Item.Meta.Id)
}

// listIn calls GetCommentList against an arbitrary budget (the clone target),
// as opposed to list, which is always h.budgetID.
func (h *commentHarness) listIn(t *testing.T, budgetID, userID vo.Id, from, months string) *model.GetCommentListResult {
	t.Helper()
	res, err := h.svc.GetCommentList(h.ctx, userID, model.GetCommentListRequest{
		BudgetId: budgetID.String(), From: from, Months: months,
	})
	if err != nil {
		t.Fatalf("GetCommentList: %v", err)
	}
	return res
}

// backdateComment overwrites a comment's stored created_at/updated_at
// directly, bypassing CreateComment's real-clock stamp. A clone test asserting
// timestamp preservation needs the source comment's timestamps to be
// unambiguously distinct from the clone's own `now`, or a regression that
// re-stamps the copy could pass by wall-clock coincidence.
func (h *commentHarness) backdateComment(t *testing.T, id string, at time.Time) {
	t.Helper()
	query := h.tdb.Rebind("UPDATE budgets_elements_comments SET created_at = ?, updated_at = ? WHERE id = ?")
	if _, err := h.tdb.Raw.ExecContext(h.ctx, query, at, at, id); err != nil {
		t.Fatalf("backdateComment: %v", err)
	}
}

// mergeCategory drives MergeService the way merge_test.go does, but over the
// same repo and clock backing the harness's own Service, so the repointed
// rows land in the database list/listIn read back from.
func (h *commentHarness) mergeCategory(t *testing.T, srcSlug, dstSlug string) {
	t.Helper()
	merger := appbudget.NewMergeService(h.budgetRepo, h.budgetRepo, h.budgetRepo, h.clk)
	src := vo.MustParseId(h.elementExternalID(t, srcSlug))
	dst := vo.MustParseId(h.elementExternalID(t, dstSlug))
	if err := merger.MergeElements(h.ctx, src, dst); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}
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

// The cap is applied in SQL: the list returns exactly 2000 rows and flags the
// overflow from the one extra row it asked for, never reading the rest.
func TestGetCommentList_CapSetsTruncated(t *testing.T) {
	h := newCommentHarness(t)
	seed := h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "seed")

	tx, err := h.tdb.Raw.BeginTx(h.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	query := h.tdb.Rebind(`INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at)
		SELECT ?, element_id, period, user_id, comment, created_at, updated_at FROM budgets_elements_comments WHERE id = ?`)
	for range 2000 {
		if _, err := tx.ExecContext(h.ctx, query, vo.NewId().String(), seed.Item.Id); err != nil {
			_ = tx.Rollback()
			t.Fatalf("seed copy: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 2000 || !res.Truncated {
		t.Fatalf("items=%d truncated=%v want 2000 and true over 2001 rows", len(res.Items), res.Truncated)
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

	res, err := h.create(t, h.guest, h.newID(), "cat-food", "2026-05-01", "  Trip to Lisbon  ")
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

	first, err := h.create(t, h.owner, id, "cat-food", "2026-05-01", "double tap")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.create(t, h.owner, id, "cat-food", "2026-05-01", "double tap")
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

	// Asserting the field, not just err != nil: an unrelated failure (e.g. a
	// broken element lookup) must not masquerade as a period-bounds rejection.
	_, err := h.create(t, h.owner, h.newID(), "cat-food", "2026-03-01", "too early")
	ve, ok := errs.AsValidation(err)
	if !ok || len(ve.Fields) != 1 || ve.Fields[0].Key != "period" {
		t.Fatalf("err=%+v want a validation error on field period", ve)
	}

	h.setEndMonth(t, "2026-07-01")
	_, err = h.create(t, h.owner, h.newID(), "cat-food", "2026-08-01", "too late")
	ve, ok = errs.AsValidation(err)
	if !ok || len(ve.Fields) != 1 || ve.Fields[0].Key != "period" {
		t.Fatalf("err=%+v want a validation error on field period", ve)
	}
}

func TestCreateComment_ArchivedAndUncategorized(t *testing.T) {
	h := newCommentHarness(t)

	if _, err := h.create(t, h.owner, h.newID(), model.UncategorizedID, "2026-05-01", "nope"); err == nil {
		t.Fatal("uncategorized accepted a comment")
	}
	h.archive(t)
	_, err := h.create(t, h.owner, h.newID(), "cat-food", "2026-05-01", "nope")
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetArchived {
		t.Fatalf("err=%v want budget.archived", err)
	}
}

// The design spec calls out at length that archiving blocks deletion too — a
// thread on an archived budget can never be cleaned up, deliberately, so an
// owner/admin "tidy up" convenience must not be added to DeleteComment without
// removing this guard on purpose. Create the comment before archiving: the
// create path is already blocked once archived (asserted above), so seeding
// it archived would never reach the code this test exists to cover.
func TestUpdateComment_Archived(t *testing.T) {
	h := newCommentHarness(t)
	own := h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "mine")

	h.archive(t)
	_, err := h.update(h.owner, own.Item.Id, "edited after archive")
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetArchived {
		t.Fatalf("err=%v want budget.archived", err)
	}
}

func TestDeleteComment_Archived(t *testing.T) {
	h := newCommentHarness(t)
	own := h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "mine")

	h.archive(t)
	_, err := h.remove(h.owner, own.Item.Id)
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetArchived {
		t.Fatalf("err=%v want budget.archived", err)
	}
}

// MINOR 5: the create side owns the "who may post" permission model
// (canRead — any accepted participant, guest included); a pending invitee and
// a stranger must be refused just like the read side already asserts.
func TestCreateComment_PermissionDenied(t *testing.T) {
	h := newCommentHarness(t)

	if _, err := h.create(t, h.pending, h.newID(), "cat-food", "2026-05-01", "nope"); err == nil {
		t.Fatal("a pending invitee posted a comment")
	}
	if _, err := h.create(t, h.stranger, h.newID(), "cat-food", "2026-05-01", "nope"); err == nil {
		t.Fatal("a stranger posted a comment")
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

	// MINOR 6: canDelete also allows an admin (not just the owner); a
	// separate harness user with the admin role exercises that half.
	byAdmin := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "moderated by admin")
	if _, err := h.remove(h.admin, byAdmin.Item.Id); err != nil {
		t.Fatalf("admin moderation rejected: %v", err)
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
		name     string
		wantCode string
		run      func() error
	}{
		{"update", errs.CodeBudgetCommentNotFound, func() error { _, err := h.update(h.owner, foreign, "peek"); return err }},
		{"delete", errs.CodeBudgetCommentNotFound, func() error { _, err := h.remove(h.owner, foreign); return err }},
		{"unknown-id", errs.CodeBudgetCommentNotFound, func() error { _, err := h.update(h.owner, h.newID().String(), "peek"); return err }},
		// CRITICAL 1: the operation guard's claimed-id table is global and
		// keyed on id alone, so "retrying" a create with someone else's
		// comment id (seen, say, in a get-comment-list response before access
		// was revoked) must not echo their row back just because the caller
		// now names their own budget/element/period. It must fail closed as a
		// locked operation, disclosing nothing about the foreign row.
		{"create-retry", errs.CodeOperationLocked, func() error {
			_, err := h.create(t, h.owner, vo.MustParseId(foreign), "cat-food", "2026-05-01", "peek")
			return err
		}},
	} {
		ve, ok := errs.AsValidation(tc.run())
		if !ok || ve.MsgCode != tc.wantCode {
			t.Fatalf("%s: want code %s, got %+v", tc.name, tc.wantCode, ve)
		}
	}
}
