package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/admin"
	adminapi "github.com/econumo/econumo/internal/admin/api"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
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

var selfID = vo.NewId()

func newMux() http.Handler {
	users := &stubUsers{byID: map[string]admin.UserRecord{
		selfID.String(): {ID: selfID.String(), Name: "Alex", Email: "alex@example.test", AccessLevel: model.AccessLevelFull},
	}}
	svc := admin.NewService(users, &stubConns{ids: map[string][]vo.Id{}}, testClock{})
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
