package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	affU1 = "a1000000-0000-0000-0000-000000000001"
	affU2 = "a1000000-0000-0000-0000-000000000002"
	affU3 = "a1000000-0000-0000-0000-000000000003"
	affU4 = "a1000000-0000-0000-0000-000000000004"
	affU5 = "a1000000-0000-0000-0000-000000000005"
)

func newAffiliateAdminHandler(t *testing.T) http.Handler {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	base := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	seed := []struct {
		id, program, code string
		at                time.Time
	}{
		// u2 and u1 share a created_at: the user id breaks the tie.
		{affU2, "a", "code-b", base},
		{affU1, "a", "code-a", base},
		{affU3, "a", "code-c", base.Add(time.Hour)},
		{affU5, "b", "code-d", base},
		{affU4, "", "", base},
	}
	for i, s := range seed {
		f.User(fixture.User{ID: s.id, Email: s.id[len(s.id)-4:] + "@example.test", Name: "U"})
		if s.code == "" {
			continue
		}
		seedAffiliate(t, db, i, s.id, s.program, s.code, s.at)
	}
	f.Connect(affU1, affU3)
	return buildAdminHandler(t, db)
}

func seedAffiliate(t *testing.T, db *dbtest.DB, i int, userID, program, code string, at time.Time) {
	t.Helper()
	for j, o := range [][2]string{{"affiliate", program}, {"affiliate_code", code}} {
		if _, err := db.Raw.Exec(db.Rebind(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`),
			"b1000000-0000-0000-0000-0000000000"+string(rune('0'+i))+string(rune('0'+j)), userID, o[0], o[1], at, at); err != nil {
			t.Fatal(err)
		}
	}
}

func buildAdminHandler(t *testing.T, db *dbtest.DB) http.Handler {
	t.Helper()
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AdminToken: adminToken}
	_, h, _, err := server.Build(cfg, db.Raw, server.Seams{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func affiliateUsers(t *testing.T, h http.Handler, query string) []model.AdminAffiliateUser {
	t.Helper()
	rec := adminDo(t, h, http.MethodGet, "/admin/affiliate-users?"+query, adminToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data []model.AdminAffiliateUser `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func TestListAffiliateUsers(t *testing.T) {
	h := newAffiliateAdminHandler(t)

	got := affiliateUsers(t, h, "program=a&since=2026-01-01+00:00:00")
	want := []model.AdminAffiliateUser{
		{UserId: affU1, Affiliate: "a", AffiliateCode: "code-a", CreatedAt: "2026-11-01 10:00:00"},
		{UserId: affU2, Affiliate: "a", AffiliateCode: "code-b", CreatedAt: "2026-11-01 10:00:00"},
		{UserId: affU3, Affiliate: "a", AffiliateCode: "code-c", CreatedAt: "2026-11-01 11:00:00"},
	}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	if got := affiliateUsers(t, h, "program=b&since=2026-01-01+00:00:00"); len(got) != 1 || got[0].UserId != affU5 || got[0].Affiliate != "b" || got[0].AffiliateCode != "code-d" {
		t.Fatalf("program b: %+v", got)
	}
	if got := affiliateUsers(t, h, "program=c&since=2026-01-01+00:00:00"); got == nil || len(got) != 0 {
		t.Fatalf("unknown program must be [], got %#v", got)
	}

	// No program means every program, in created_at then user id order.
	all := affiliateUsers(t, h, "since=2026-01-01+00:00:00")
	if len(all) != 4 || all[0].UserId != affU1 || all[1].UserId != affU2 || all[2].UserId != affU5 || all[3].UserId != affU3 {
		t.Fatalf("no program: %+v", all)
	}
	if got := affiliateUsers(t, h, "program=&since=2026-01-01+00:00:00"); len(got) != 4 {
		t.Fatalf("empty program: %+v", got)
	}

	// since is inclusive.
	if got := affiliateUsers(t, h, "program=a&since=2026-11-01+11:00:00"); len(got) != 1 || got[0].UserId != affU3 {
		t.Fatalf("since filter: %+v", got)
	}
	if got := affiliateUsers(t, h, "program=a&since=2026-11-01+11:00:01"); got == nil || len(got) != 0 {
		t.Fatalf("empty result must be [], got %#v", got)
	}
	if got := affiliateUsers(t, h, "program=a&since=2026-01-01+00:00:00&limit=2"); len(got) != 2 || got[1].UserId != affU2 {
		t.Fatalf("limit: %+v", got)
	}
}

func TestListAffiliateUsersPagesThroughSameSecondTies(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{affU1, affU2, affU3} {
		f.User(fixture.User{ID: id, Email: id[len(id)-4:] + "@example.test", Name: "U"})
		seedAffiliate(t, db, i, id, "a", "code", at)
	}
	h := buildAdminHandler(t, db)

	first := affiliateUsers(t, h, "program=a&since=2026-11-01+10:00:00&limit=2")
	if len(first) != 2 || first[0].UserId != affU1 || first[1].UserId != affU2 {
		t.Fatalf("first page: %+v", first)
	}
	second := affiliateUsers(t, h, "program=a&since="+strings.ReplaceAll(first[1].CreatedAt, " ", "+")+"&afterUserId="+first[1].UserId+"&limit=2")
	if len(second) != 1 || second[0].UserId != affU3 {
		t.Fatalf("second page: %+v", second)
	}
	firstAll := affiliateUsers(t, h, "since=2026-11-01+10:00:00&limit=2")
	secondAll := affiliateUsers(t, h, "since="+strings.ReplaceAll(firstAll[1].CreatedAt, " ", "+")+"&afterUserId="+firstAll[1].UserId+"&limit=2")
	if len(firstAll) != 2 || len(secondAll) != 1 || secondAll[0].UserId != affU3 {
		t.Fatalf("paging without program: %+v then %+v", firstAll, secondAll)
	}
	if rec := adminDo(t, h, http.MethodGet, "/admin/affiliate-users?program=a&since=2026-11-01+10:00:00&afterUserId=nope", adminToken, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad afterUserId: status = %d, want 400", rec.Code)
	}
}

func TestListAffiliateUsersValidation(t *testing.T) {
	h := newAffiliateAdminHandler(t)
	for _, q := range []string{
		"", "program=a", "program=A&since=2026-01-01+00:00:00",
		"program=a+b&since=2026-01-01+00:00:00",
		"program=a&since=garbage", "program=a&since=2026-01-01",
		"program=a&since=2026-01-01+00:00:00&limit=0",
		"program=a&since=2026-01-01+00:00:00&limit=501",
		"program=a&since=2026-01-01+00:00:00&limit=x",
	} {
		if rec := adminDo(t, h, http.MethodGet, "/admin/affiliate-users?"+q, adminToken, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", q, rec.Code)
		}
	}
	if rec := adminDo(t, h, http.MethodGet, "/admin/affiliate-users?program=a&since=2026-01-01+00:00:00", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no bearer: status = %d, want 401", rec.Code)
	}
}

func TestAdminUserContextIncludesAffiliate(t *testing.T) {
	h := newAffiliateAdminHandler(t)
	if got := adminData(t, adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+affU1, adminToken, "")); got.User.Affiliate != "a" || got.User.AffiliateCode != "code-a" {
		t.Fatalf("affiliate = %q %q", got.User.Affiliate, got.User.AffiliateCode)
	}
	if got := adminData(t, adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+affU1, adminToken, "")); len(got.Connections) != 1 || got.Connections[0].Id != affU3 || got.Connections[0].Affiliate != "a" || got.Connections[0].AffiliateCode != "code-c" {
		t.Fatalf("connections = %+v", got.Connections)
	}
	rec := adminDo(t, h, http.MethodGet, "/admin/user-context?userId="+affU4, adminToken, "")
	if got := adminData(t, rec); got.User.Affiliate != "" || got.User.AffiliateCode != "" {
		t.Fatalf("affiliate = %q", got.User.Affiliate)
	}
	if !strings.Contains(rec.Body.String(), `"affiliate":""`) || !strings.Contains(rec.Body.String(), `"affiliateCode":""`) {
		t.Fatalf("field missing: %s", rec.Body.String())
	}
}
