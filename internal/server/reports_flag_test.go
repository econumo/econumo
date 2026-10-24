package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func TestReportRoutesFollowFlag(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[on], func(t *testing.T) { checkReportRoutes(t, on) })
	}
}

func checkReportRoutes(t *testing.T, on bool) {
	{
		db := dbtest.NewSQLite(t)
		f := fixture.New(t, db)
		userID := f.User(fixture.User{})
		f.DefaultOptions(userID)
		exp := time.Now().UTC().Add(24 * time.Hour)
		token := "eco_pat_reports-flag-test-full-00000000000000000000"
		f.AccessToken(fixture.AccessToken{UserID: userID, Kind: "personal", TokenHash: appuser.HashAccessToken(token), ExpiresAt: &exp})

		cfg := baseTestConfig(db.Engine)
		cfg.Reports = on
		ts := httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))

		for _, path := range []string{"/api/v1/report/get-flow-report", "/api/v1/report/get-net-worth-report"} {
			req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			want := http.StatusNotFound
			if on {
				want = http.StatusOK
			}
			if resp.StatusCode != want {
				t.Errorf("reports=%v %s: status %d, want %d", on, path, resp.StatusCode, want)
			}
		}
		ts.Close()
	}
}
