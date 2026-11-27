package user_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func inactivityRequest(t *testing.T, body string) model.UpdateInactivityPeriodRequest {
	t.Helper()
	var req model.UpdateInactivityPeriodRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return req
}

// updateInactivity runs the request the way the endpoint does: Validate, then
// the use case.
func updateInactivity(e *passEnv, id vo.Id, req model.UpdateInactivityPeriodRequest) (*model.UpdateInactivityPeriodResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return e.svc.UpdateInactivityPeriod(context.Background(), id, req)
}

func inactivityRows(t *testing.T, e *passEnv, id vo.Id) []string {
	t.Helper()
	opts, err := e.users.GetOptions(context.Background(), id)
	if err != nil {
		t.Fatalf("GetOptions: %v", err)
	}
	var out []string
	for _, o := range opts {
		if o.Name == model.OptionInactivityMonths {
			v := "<nil>"
			if o.Value != nil {
				v = *o.Value
			}
			out = append(out, o.ID.String()+"="+v)
		}
	}
	return out
}

func TestUpdateInactivityPeriod_Choices(t *testing.T) {
	e := newPassEnv(t, inactive12)
	u := e.user(t, "me@pass.test", 0)

	for _, months := range []string{"6", "12", "24"} {
		if _, err := updateInactivity(e, u, inactivityRequest(t, `{"months": `+months+`}`)); err != nil {
			t.Errorf("months %s refused: %v", months, err)
		}
	}
	for _, body := range []string{`{"months": 3}`, `{"months": 5}`, `{"months": 36}`, `{"months": 0}`, `{}`, `{"months": 6.5}`, `{"months": "6"}`} {
		_, err := updateInactivity(e, u, inactivityRequest(t, body))
		verr, ok := errs.AsValidation(err)
		if !ok || len(verr.Fields) != 1 || verr.Fields[0].Key != "months" || verr.Fields[0].Message != "This value is not valid." {
			t.Errorf("%s: err = %#v, want the months validation error", body, err)
		}
	}
}

func TestUpdateInactivityPeriod_RefusedWhileOff(t *testing.T) {
	e := newPassEnv(t, appuser.DeletionConfig{InactiveMonths: 0, DeletionDays: 30})
	u := e.user(t, "me@pass.test", 0)
	_, err := updateInactivity(e, u, inactivityRequest(t, `{"months": 12}`))
	if verr, ok := errs.AsValidation(err); !ok || verr.Fields[0].Key != "months" {
		t.Fatalf("err = %#v, want the months validation error", err)
	}
	if rows := inactivityRows(t, e, u); len(rows) != 0 {
		t.Fatalf("rows = %v, want none written", rows)
	}
}

func TestUpdateInactivityPeriod_WritesOneRow(t *testing.T) {
	e := newPassEnv(t, inactive12)
	u := e.user(t, "me@pass.test", 0)

	res, err := updateInactivity(e, u, inactivityRequest(t, `{"months": 6}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.User.InactivityMonths != 6 || !slices.Equal(res.User.InactivityChoices, []int{6, 12, 24}) {
		t.Fatalf("user = %d %v, want 6 [6 12 24]", res.User.InactivityMonths, res.User.InactivityChoices)
	}
	first := inactivityRows(t, e, u)
	if len(first) != 1 {
		t.Fatalf("rows = %v, want one", first)
	}
	if _, err := updateInactivity(e, u, inactivityRequest(t, `{"months": 24}`)); err != nil {
		t.Fatal(err)
	}
	second := inactivityRows(t, e, u)
	if len(second) != 1 || second[0][:36] != first[0][:36] || second[0][37:] != "24" {
		t.Fatalf("rows = %v after %v, want the same row holding 24", second, first)
	}
}

func TestCurrentUser_Inactivity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		server      int
		option      string
		wantMonths  int
		wantChoices []int
	}{
		{"off with an option", 0, "24", 0, []int{}},
		{"default", 12, "", 12, []int{6, 12, 24}},
		{"own choice", 12, "24", 24, []int{6, 12, 24}},
		{"seeded lifetime", 12, "36", 36, []int{6, 12, 24}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPassEnv(t, appuser.DeletionConfig{InactiveMonths: tc.server, DeletionDays: 30})
			u := e.user(t, "me@pass.test", 0)
			if tc.option != "" {
				e.setInactivityMonths(t, u, tc.option)
			}
			check := func(path string, cur model.CurrentUserResult) {
				t.Helper()
				if cur.InactivityMonths != tc.wantMonths || cur.InactivityChoices == nil || !slices.Equal(cur.InactivityChoices, tc.wantChoices) {
					t.Errorf("%s: %d %#v, want %d %v", path, cur.InactivityMonths, cur.InactivityChoices, tc.wantMonths, tc.wantChoices)
				}
			}

			// The write path's view (every update-* response).
			res, err := e.svc.UpdateAI(context.Background(), u, model.UpdateAIRequest{Enabled: new(true)})
			if err != nil {
				t.Fatal(err)
			}
			check("write path", res.User)

			read := appuser.NewReadService(userrepo.NewReadRepo(e.db.Engine, e.db.TX), auth.NewEncodeService(""), e.clk)
			read.SetInactiveMonths(tc.server)
			data, err := read.GetUserData(context.Background(), u)
			if err != nil {
				t.Fatal(err)
			}
			check("get-user-data", data.User)
		})
	}
}

func TestSeedLifetimeInactivity(t *testing.T) {
	e := newPassEnv(t, inactive12)
	ctx := context.Background()
	lifetime := e.user(t, "lifetime@pass.test", 0)
	chosen := e.user(t, "chosen@pass.test", 0)
	e.setInactivityMonths(t, chosen, "6")
	trial := e.user(t, "trial@pass.test", 0)
	e.db.Exec(t, e.db.Rebind(`UPDATE users SET access_until = ? WHERE id = ?`), e.clk.now.AddDate(0, 0, 14), trial.String())
	readonly := e.user(t, "readonly@pass.test", 0)
	e.db.Exec(t, e.db.Rebind(`UPDATE users SET access_level = 'readonly' WHERE id = ?`), readonly.String())

	n, err := e.svc.SeedLifetimeInactivity(ctx)
	if err != nil || n != 1 {
		t.Fatalf("seed = %d, %v; want 1", n, err)
	}
	value := func(id vo.Id) string {
		rows := inactivityRows(t, e, id)
		if len(rows) == 0 {
			return ""
		}
		return rows[0][37:]
	}
	for id, want := range map[vo.Id]string{lifetime: "36", chosen: "6", trial: "", readonly: ""} {
		if got := value(id); got != want {
			t.Errorf("%s: inactivity_months = %q, want %q", id, got, want)
		}
	}
	if n, err := e.svc.SeedLifetimeInactivity(ctx); err != nil || n != 0 {
		t.Fatalf("second seed = %d, %v; want 0", n, err)
	}
}
