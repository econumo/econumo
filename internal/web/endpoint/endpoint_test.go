package endpoint_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/middleware"
)

// testReq is a minimal request DTO implementing httpx.Validator (tier-1
// validation): Bad=true simulates a request that decodes fine but fails
// validation, the same shape every real request DTO uses.
type testReq struct {
	Bad bool `json:"bad"`
}

func (r *testReq) Validate() error {
	if r.Bad {
		return errs.NewValidation("bad request", errs.FieldError{Key: "bad", Message: "must be false"})
	}
	return nil
}

type testRes struct {
	Value string `json:"value"`
}

// authedRequest builds a GET/POST request already carrying a verified user in
// its context, the same way the real auth middleware does. Handle/HandleNoBody
// read the user id via middleware.RequireUser, whose context key is
// unexported, so routing a token through middleware.Auth is the only exported
// way to populate it from outside the middleware package (authstub: the
// bearer token IS the user id string).
func authedRequest(t *testing.T, method string, body []byte) (*http.Request, *httptest.ResponseRecorder, func(http.Handler) http.Handler) {
	t.Helper()
	token := "11111111-1111-1111-1111-111111111111"

	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/x", rdr)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	return req, rec, middleware.Auth(authstub.Authenticator{}, false)
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, rec.Body.String())
	}
	return env
}

func TestHandle_NoUserYields401(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(`{}`)))
	// No JWT middleware wrapping -> no user in context.
	endpoint.Handle(rec, req, false, func(ctx context.Context, userID vo.Id, r testReq) (testRes, error) {
		t.Fatal("call must not run without an authenticated user")
		return testRes{}, nil
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	env := decodeEnvelope(t, rec)
	if env["success"] != false {
		t.Fatalf("success = %v, want false", env["success"])
	}
}

func TestHandle_ValidationFailureYields400(t *testing.T) {
	req, rec, jwtMw := authedRequest(t, http.MethodPost, []byte(`{"bad":true}`))
	handler := jwtMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.Handle(w, r, false, func(ctx context.Context, userID vo.Id, req testReq) (testRes, error) {
			t.Fatal("call must not run when validation fails")
			return testRes{}, nil
		})
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env["success"] != false {
		t.Fatalf("success = %v, want false", env["success"])
	}
}

func TestHandle_ServiceErrorGoesThroughWriteError(t *testing.T) {
	req, rec, jwtMw := authedRequest(t, http.MethodPost, []byte(`{"bad":false}`))
	wantErr := errors.New("boom")
	handler := jwtMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.Handle(w, r, false, func(ctx context.Context, userID vo.Id, req testReq) (testRes, error) {
			return testRes{}, wantErr
		})
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (unmapped error -> exception envelope); body: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env["success"] != false {
		t.Fatalf("success = %v, want false; body: %s", env["success"], rec.Body.String())
	}
	// dev=false: the unmapped error goes through the exception envelope with a
	// generic message; the raw error text must not leak to the client.
	if env["message"] != "Internal Server Error" {
		t.Fatalf("message = %v, want generic; body: %s", env["message"], rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), wantErr.Error()) {
		t.Fatalf("prod 500 body leaked raw error %q: %s", wantErr.Error(), rec.Body.String())
	}
}

func TestHandle_SuccessYieldsOKEnvelope(t *testing.T) {
	req, rec, jwtMw := authedRequest(t, http.MethodPost, []byte(`{"bad":false}`))
	var gotUserID vo.Id
	handler := jwtMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.Handle(w, r, false, func(ctx context.Context, userID vo.Id, req testReq) (testRes, error) {
			gotUserID = userID
			return testRes{Value: "ok"}, nil
		})
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotUserID.String() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("userID = %q, want the JWT subject", gotUserID.String())
	}
	env := decodeEnvelope(t, rec)
	if env["success"] != true {
		t.Fatalf("success = %v, want true", env["success"])
	}
	data, _ := env["data"].(map[string]any)
	if data["value"] != "ok" {
		t.Fatalf("data.value = %v, want ok", data["value"])
	}
}

func TestHandleNoBody_NoUserYields401(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	endpoint.HandleNoBody(rec, req, false, func(ctx context.Context, userID vo.Id) (testRes, error) {
		t.Fatal("call must not run without an authenticated user")
		return testRes{}, nil
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandleNoBody_ServiceErrorGoesThroughWriteError(t *testing.T) {
	req, rec, jwtMw := authedRequest(t, http.MethodGet, nil)
	handler := jwtMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.HandleNoBody(w, r, false, func(ctx context.Context, userID vo.Id) (testRes, error) {
			return testRes{}, errs.NewAccessDenied("")
		})
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleNoBody_SuccessYieldsOKEnvelope(t *testing.T) {
	req, rec, jwtMw := authedRequest(t, http.MethodGet, nil)
	handler := jwtMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.HandleNoBody(w, r, false, func(ctx context.Context, userID vo.Id) (testRes, error) {
			return testRes{Value: "listed"}, nil
		})
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env["success"] != true {
		t.Fatalf("success = %v, want true", env["success"])
	}
}

func TestHandlePublic_ValidationFailureYields400(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(`{"bad":true}`)))
	endpoint.HandlePublic(rec, req, false, func(ctx context.Context, req testReq) (testRes, error) {
		t.Fatal("call must not run when validation fails")
		return testRes{}, nil
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePublic_ServiceErrorGoesThroughWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(`{"bad":false}`)))
	endpoint.HandlePublic(rec, req, false, func(ctx context.Context, req testReq) (testRes, error) {
		return testRes{}, errs.NewUnauthorized("nope")
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePublic_SuccessYieldsOKEnvelopeWithNoUserGate(t *testing.T) {
	rec := httptest.NewRecorder()
	// No Authorization header at all -> HandlePublic must not require one.
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(`{"bad":false}`)))
	endpoint.HandlePublic(rec, req, false, func(ctx context.Context, req testReq) (testRes, error) {
		return testRes{Value: "public-ok"}, nil
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env["success"] != true {
		t.Fatalf("success = %v, want true", env["success"])
	}
}
