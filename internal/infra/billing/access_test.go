package billing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
)

func signStub(uid string) (string, error) { return "tok-" + uid, nil }

func newClient(t *testing.T, base string, hc *http.Client) *AccessClient {
	t.Helper()
	c, err := NewAccessClient(base, signStub, hc)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const okBody = `{"success":true,"message":"","data":{` +
	`"user":{"paid":true,"renewing":true,"renewsAt":"2026-12-24 10:00:00","endsAt":"","paymentFailed":false,"discountUntil":"2027-11-24 10:00:00"},` +
	`"connections":[{"id":"c1","paid":true,"renewing":true}]}}`

func TestGetAccessSendsTheContractAndMapsTheAnswer(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotType, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotPath, gotQuery, gotAuth, gotType, gotBody = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(raw)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = io.WriteString(w, okBody)
	}))
	defer ts.Close()

	u, conns, err := newClient(t, ts.URL+"/cloud/?tenant=x", nil).GetAccess(context.Background(), "u1", []string{"c1", "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/cloud/api/v1/econumo/get-access" || gotQuery != "tenant=x" {
		t.Fatalf("url = %s?%s", gotPath, gotQuery)
	}
	if gotAuth != "Bearer tok-u1" || gotType != "application/json" {
		t.Fatalf("auth = %q, content-type = %q", gotAuth, gotType)
	}
	if gotBody != `{"connections":["c1","c2"]}` {
		t.Fatalf("body = %s", gotBody)
	}
	want := model.BillingAccessUser{Paid: true, Renewing: true, RenewsAt: "2026-12-24 10:00:00", DiscountUntil: "2027-11-24 10:00:00"}
	if u != want || len(conns) != 1 || conns[0] != (model.BillingAccessConnection{Id: "c1", Paid: true, Renewing: true}) {
		t.Fatalf("got %+v %+v", u, conns)
	}
}

func TestGetAccessSendsAnEmptyListWithoutConnections(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = io.WriteString(w, okBody)
	}))
	defer ts.Close()
	if _, _, err := newClient(t, ts.URL, nil).GetAccess(context.Background(), "u1", nil); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"connections":[]}` {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestGetAccessRejectsWhatIsNotAWellFormedView(t *testing.T) {
	for name, answer := range map[string]struct {
		status int
		body   string
	}{
		"older billing without the route": {http.StatusNotFound, `404 page not found`},
		"error envelope":                  {http.StatusOK, `{"success":false,"message":"nope","code":0,"errors":{}}`},
		"server error":                    {http.StatusInternalServerError, okBody},
		"not json":                        {http.StatusOK, `<html>`},
		"no data":                         {http.StatusOK, `{"success":true,"message":""}`},
		"no user":                         {http.StatusOK, `{"success":true,"message":"","data":{"connections":[]}}`},
		"malformed date": {http.StatusOK, `{"success":true,"data":{"user":{"paid":true,"renewing":true,"renewsAt":"2026-12-24T10:00:00Z",` +
			`"endsAt":"","paymentFailed":false,"discountUntil":""},"connections":[]}}`},
		"wrong type":            {http.StatusOK, `{"success":true,"data":{"user":{"paid":"yes"}}}`},
		"incomplete user":       {http.StatusOK, `{"success":true,"message":"","data":{"user":{},"connections":[]}}`},
		"user without renewing": {http.StatusOK, `{"success":true,"data":{"user":{"paid":true,"renewsAt":"","endsAt":"","paymentFailed":false,"discountUntil":""},"connections":[]}}`},
		"no connections": {http.StatusOK, `{"success":true,"data":{"user":{"paid":true,"renewing":false,"renewsAt":"","endsAt":"",` +
			`"paymentFailed":false,"discountUntil":""}}}`},
		"incomplete connection": {http.StatusOK, `{"success":true,"data":{"user":{"paid":true,"renewing":false,"renewsAt":"","endsAt":"",` +
			`"paymentFailed":false,"discountUntil":""},"connections":[{"id":"c1"}]}}`},
	} {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(answer.status)
				_, _ = io.WriteString(w, answer.body)
			}))
			defer ts.Close()
			_, _, err := newClient(t, ts.URL, nil).GetAccess(context.Background(), "u1", nil)
			if !errors.Is(err, ErrBadResponse) {
				t.Fatalf("err = %v, want ErrBadResponse", err)
			}
		})
	}
}

func TestGetAccessTimesOut(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer ts.Close()
	defer close(release)
	_, _, err := newClient(t, ts.URL, &http.Client{Timeout: 50 * time.Millisecond}).GetAccess(context.Background(), "u1", nil)
	var timeout interface{ Timeout() bool }
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("err = %v, want a timeout", err)
	}
}
