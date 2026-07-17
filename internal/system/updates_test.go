package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

func feedServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func info(t *testing.T, s *Service) (string, string) {
	t.Helper()
	res, err := s.GetUpdateInfo(context.Background(), vo.NewId())
	if err != nil {
		t.Fatal(err)
	}
	return res.Version, res.Url
}

func TestFetchValidPayload(t *testing.T) {
	srv := feedServer(t, 200, `{"version":"v9.9.9","date":"2026-07-16","url":"https://econumo.com/releases/v9.9.9/"}`)
	s := NewService(true, srv.URL)
	s.fetch(context.Background())
	v, u := info(t, s)
	if v != "v9.9.9" || u != "https://econumo.com/releases/v9.9.9/" {
		t.Fatalf("info = %q %q, want v9.9.9 + release url", v, u)
	}
}

func TestFetchRejectsInvalidPayloads(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"malformed json":   {200, `{"version": `},
		"bad version":      {200, `{"version":"latest","url":"https://econumo.com/releases/latest/"}`},
		"wrong url origin": {200, `{"version":"v9.9.9","url":"https://evil.example/phish/"}`},
		"non-2xx":          {500, `{"version":"v9.9.9","url":"https://econumo.com/releases/v9.9.9/"}`},
		"empty payload":    {200, `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := feedServer(t, tc.status, tc.body)
			s := NewService(true, srv.URL)
			s.fetch(context.Background())
			if v, u := info(t, s); v != "" || u != "" {
				t.Fatalf("info = %q %q, want empty (payload must be dropped)", v, u)
			}
		})
	}
}

func TestFetchFailureKeepsPreviousValue(t *testing.T) {
	good := feedServer(t, 200, `{"version":"v9.9.9","url":"https://econumo.com/releases/v9.9.9/"}`)
	s := NewService(true, good.URL)
	s.fetch(context.Background())

	bad := feedServer(t, 500, ``)
	s.feedURL = bad.URL
	s.fetch(context.Background())
	if v, _ := info(t, s); v != "v9.9.9" {
		t.Fatalf("version after failed refetch = %q, want v9.9.9 retained", v)
	}
}

func TestStartPollingDisabledIsNoop(t *testing.T) {
	srv := feedServer(t, 200, `{"version":"v9.9.9","url":"https://econumo.com/releases/v9.9.9/"}`)
	s := NewService(false, srv.URL)
	s.StartPolling(context.Background())
	if v, _ := info(t, s); v != "" {
		t.Fatalf("disabled service has version %q, want empty", v)
	}
}
