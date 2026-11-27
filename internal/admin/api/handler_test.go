package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/admin"
	adminapi "github.com/econumo/econumo/internal/admin/api"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

var now = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

type testClock struct{}

func (testClock) Now() time.Time { return now }

type stubUsers struct{ byID map[string]admin.UserRecord }

func (s *stubUsers) GetUser(_ context.Context, id vo.Id) (admin.UserRecord, error) {
	u, ok := s.byID[id.String()]
	if !ok {
		return admin.UserRecord{}, errs.NewNotFound("User not found")
	}
	return u, nil
}

func (s *stubUsers) SetAccess(_ context.Context, id vo.Id, level model.AccessLevel, until *time.Time) (admin.UserRecord, error) {
	rec, ok := s.byID[id.String()]
	if !ok {
		return admin.UserRecord{}, errs.NewNotFound("User not found")
	}
	rec.AccessLevel, rec.AccessUntil = level, until
	s.byID[id.String()] = rec
	return rec, nil
}

type stubConns struct{ ids map[string][]vo.Id }

func (s *stubConns) ConnectedUserIDs(_ context.Context, id vo.Id) ([]vo.Id, error) {
	return s.ids[id.String()], nil
}

type noDeletions struct{}

func (noDeletions) GetState(context.Context, vo.Id) (model.DeletionState, error) {
	return model.DeletionState{}, nil
}

var selfID = vo.NewId()

func newMux() http.Handler {
	users := &stubUsers{byID: map[string]admin.UserRecord{
		selfID.String(): {ID: selfID.String(), Name: "Alex", Email: "alex@example.test", AccessLevel: model.AccessLevelFull},
	}}
	svc := admin.NewService(users, &stubConns{ids: map[string][]vo.Id{}}, nil, nil, noDeletions{}, testClock{})
	mux := http.NewServeMux()
	adminapi.RegisterAdmin(adminapi.NewHandlers(svc))(mux)
	return mux
}

func do(method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, r)
	return rec
}

func TestSetAccessHandlerOK(t *testing.T) {
	rec := do(http.MethodPost, "/admin/set-access",
		`{"userId":"`+selfID.String()+`","level":"readonly","until":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool                `json:"success"`
		Data    model.AdminUserView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Success || env.Data.AccessLevel != "readonly" || env.Data.AccessUntil != "" {
		t.Fatalf("data = %+v", env.Data)
	}
}

func TestSetAccessHandlerRejectsBadLevel(t *testing.T) {
	rec := do(http.MethodPost, "/admin/set-access",
		`{"userId":"`+selfID.String()+`","level":"premium"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// A machine consumer needs "no such user" (stop retrying) to be distinguishable
// from "malformed request" (fix your code).
func TestSetAccessHandlerUnknownUserIs404(t *testing.T) {
	rec := do(http.MethodPost, "/admin/set-access",
		`{"userId":"`+vo.NewId().String()+`","level":"full"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUserContextHandlerRequiresUserId(t *testing.T) {
	if rec := do(http.MethodGet, "/admin/user-context", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserContextHandlerRejectsMalformedId(t *testing.T) {
	if rec := do(http.MethodGet, "/admin/user-context?userId=nope", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserContextHandlerUnknownUserIs404(t *testing.T) {
	rec := do(http.MethodGet, "/admin/user-context?userId="+vo.NewId().String(), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUserContextHandlerReturnsEnvelope(t *testing.T) {
	rec := do(http.MethodGet, "/admin/user-context?userId="+selfID.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool                         `json:"success"`
		Message string                       `json:"message"`
		Data    model.AdminUserContextResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Success || env.Data.User.Email != "alex@example.test" {
		t.Fatalf("envelope = %s", rec.Body.String())
	}
	if env.Data.Connections == nil {
		t.Fatal("connections must serialize as [] rather than null")
	}
}

type deletionHarness struct {
	db       *dbtest.DB
	users    *stubUsers
	audit    *userrepo.AuditRepo
	deletion *userrepo.DeletionRepo
	mux      http.Handler
}

func newDeletionHarness(t *testing.T) *deletionHarness {
	t.Helper()
	db := dbtest.New(t)
	h := &deletionHarness{
		db:       db,
		users:    &stubUsers{byID: map[string]admin.UserRecord{}},
		audit:    userrepo.NewAuditRepo(db.Engine, db.TX),
		deletion: userrepo.NewDeletionRepo(db.Engine, db.TX),
	}
	svc := admin.NewService(h.users, &stubConns{ids: map[string][]vo.Id{}}, nil, h.audit, h.deletion, testClock{})
	mux := http.NewServeMux()
	adminapi.RegisterAdmin(adminapi.NewHandlers(svc))(mux)
	h.mux = mux
	return h
}

func (h *deletionHarness) user(t *testing.T, email string) vo.Id {
	t.Helper()
	id := fixture.New(t, h.db).User(fixture.User{Email: email})
	h.users.byID[id] = admin.UserRecord{ID: id, Name: "User", Email: email, AccessLevel: model.AccessLevelFull}
	return vo.MustParseId(id)
}

func (h *deletionHarness) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func (h *deletionHarness) deletedRow(t *testing.T, at time.Time, details string) model.UserAuditEvent {
	t.Helper()
	e := model.UserAuditEvent{ID: vo.NewId(), UserID: vo.NewId(), Event: model.AuditDeleted, Details: details, CreatedAt: at}
	if _, err := h.audit.Insert(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	return e
}

func deletedPage(t *testing.T, rec *httptest.ResponseRecorder) []model.AdminDeletedUser {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data []model.AdminDeletedUser `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data == nil {
		t.Fatalf("data must serialize as [] rather than null: %s", rec.Body.String())
	}
	return env.Data
}

func TestDeletedUsersPagesWithinOneSecond(t *testing.T) {
	h := newDeletionHarness(t)
	at := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	other := h.otherAuditRow(t, at)
	want := map[string]model.UserAuditEvent{}
	for _, d := range []string{
		`{"reason":"inactive","lifetimeAccess":false}`,
		`{"reason":"self_initiated","lifetimeAccess":true}`,
		`{"reason":"cli","lifetimeAccess":false}`,
	} {
		e := h.deletedRow(t, at, d)
		want[e.ID.String()] = e
	}

	since := url.QueryEscape(at.Format(datetime.Layout))
	first := deletedPage(t, h.get(t, "/admin/deleted-users?since="+since+"&limit=2"))
	if len(first) != 2 {
		t.Fatalf("first page = %+v, want 2 rows", first)
	}
	second := deletedPage(t, h.get(t, "/admin/deleted-users?since="+since+"&afterId="+first[1].Id))
	if len(second) != 1 {
		t.Fatalf("second page = %+v, want 1 row", second)
	}

	all := append(first, second...)
	for i, row := range all {
		e, ok := want[row.Id]
		if !ok {
			t.Fatalf("row %d = %+v is not a seeded deleted row (other event %s leaked?)", i, row, other)
		}
		delete(want, row.Id)
		if i > 0 && all[i-1].Id >= row.Id {
			t.Fatalf("rows not in id order within the second: %+v", all)
		}
		if row.UserId != e.UserID.String() || row.DeletedAt != "2026-07-20 10:00:00" {
			t.Fatalf("row = %+v, want user %s at 2026-07-20 10:00:00", row, e.UserID)
		}
		var d struct {
			Reason         string `json:"reason"`
			LifetimeAccess bool   `json:"lifetimeAccess"`
		}
		if err := json.Unmarshal([]byte(e.Details), &d); err != nil {
			t.Fatal(err)
		}
		if row.Reason != d.Reason || row.LifetimeAccess != d.LifetimeAccess {
			t.Fatalf("row = %+v, want reason %q lifetime %v", row, d.Reason, d.LifetimeAccess)
		}
	}
	if len(want) != 0 {
		t.Fatalf("rows never returned: %+v", want)
	}
}

// otherAuditRow seeds a row of another event in the same second, which the
// poll must not return.
func (h *deletionHarness) otherAuditRow(t *testing.T, at time.Time) string {
	t.Helper()
	e := model.UserAuditEvent{ID: vo.NewId(), UserID: vo.NewId(), Event: model.AuditDeletionMarked, Details: `{"reason":"inactive"}`, CreatedAt: at}
	if _, err := h.audit.Insert(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	return e.ID.String()
}

func TestDeletedUsersValidation(t *testing.T) {
	h := newDeletionHarness(t)
	for _, tc := range []struct{ query, field string }{
		{"", "since"},
		{"?since=yesterday", "since"},
		{"?since=2026-01-01+00:00:00&limit=0", "limit"},
		{"?since=2026-01-01+00:00:00&limit=501", "limit"},
		{"?since=2026-01-01+00:00:00&afterId=nope", "afterId"},
	} {
		rec := h.get(t, "/admin/deleted-users"+tc.query)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q: status = %d, want 400: %s", tc.query, rec.Code, rec.Body.String())
		}
		var env struct {
			Errors map[string][]string `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if len(env.Errors[tc.field]) == 0 {
			t.Fatalf("%q: errors = %v, want field %q", tc.query, env.Errors, tc.field)
		}
	}
}

func TestUserContextCarriesDeletionMark(t *testing.T) {
	h := newDeletionHarness(t)
	marked := h.user(t, "marked@example.test")
	unmarked := h.user(t, "unmarked@example.test")
	markedAt := time.Date(2026, 7, 19, 8, 30, 15, 0, time.UTC)
	if _, err := h.deletion.Mark(context.Background(), marked, model.DeletionReasonInactive, markedAt); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id         vo.Id
		at, reason string
	}{
		{marked, "2026-07-19 08:30:15", "inactive"},
		{unmarked, "", ""},
	} {
		rec := h.get(t, "/admin/user-context?userId="+tc.id.String())
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"deletionMarkedAt":"`+tc.at+`"`) ||
			!strings.Contains(rec.Body.String(), `"deletionReason":"`+tc.reason+`"`) {
			t.Fatalf("user %s: body = %s, want deletionMarkedAt %q, deletionReason %q", tc.id, rec.Body.String(), tc.at, tc.reason)
		}
	}
}

func TestUserContextOfWipedUserIs404(t *testing.T) {
	h := newDeletionHarness(t)
	id := h.user(t, "wiped@example.test")
	if _, err := h.deletion.DeleteUser(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	delete(h.users.byID, id.String())
	if rec := h.get(t, "/admin/user-context?userId="+id.String()); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
