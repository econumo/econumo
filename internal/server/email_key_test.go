package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"context"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
)

const emailKeyPassword = "correct-horse-battery"

type sentMail struct {
	mu   sync.Mutex
	msgs []mailer.Message
}

func (m *sentMail) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
	return nil
}

func (m *sentMail) take() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.msgs
	m.msgs = nil
	return out
}

type emailKeyHarness struct {
	t    *testing.T
	db   *dbtest.DB
	ts   *httptest.Server
	mail *sentMail
}

func newEmailKeyHarness(t *testing.T) emailKeyHarness {
	t.Helper()
	db := dbtest.New(t)
	mail := &sentMail{}
	ts := httptest.NewServer(server.BuildAPI(config.Config{
		DatabaseDriver:    db.Engine,
		CurrencyBase:      "USD",
		AllowRegistration: true,
		RateLimitWindow:   15 * time.Minute,
	}, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar), Mailer: mail}))
	t.Cleanup(ts.Close)
	return emailKeyHarness{t: t, db: db, ts: ts, mail: mail}
}

func (h emailKeyHarness) post(path string, body map[string]string) (int, string) {
	h.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	resp, err := http.Post(h.ts.URL+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp.StatusCode, string(out)
}

func (h emailKeyHarness) register(email string) (int, string) {
	h.t.Helper()
	return h.post("/api/v1/user/register-user", map[string]string{"email": email, "password": emailKeyPassword, "name": "Owner"})
}

func (h emailKeyHarness) mustRegister(email string) {
	h.t.Helper()
	if got, body := h.register(email); got != http.StatusOK {
		h.t.Fatalf("register %s = %d: %s", email, got, body)
	}
}

func (h emailKeyHarness) login(email, password string) int {
	h.t.Helper()
	got, _ := h.post("/api/v1/user/login-user", map[string]string{"username": email, "password": password})
	return got
}

var resetCode = regexp.MustCompile(`\b\d{6}\b`)

func TestRegister_RefusesAnAddressDifferingOnlyInNonASCIICase(t *testing.T) {
	cases := []struct{ first, second string }{
		{"JÖRG@example.test", "jörg@example.test"},
		{"jörg@example.test", "JÖRG@example.test"},
		{"Jörg@Example.test", "jÖRG@example.TEST"},
		{"owner@example.test", "OWNER@example.test"},
	}
	for _, c := range cases {
		t.Run(c.first+"/"+c.second, func(t *testing.T) {
			h := newEmailKeyHarness(t)
			h.mustRegister(c.first)
			if got, _ := h.register(c.second); got != http.StatusBadRequest {
				t.Fatalf("second spelling registered with %d, want 400", got)
			}
		})
	}
}

func TestPasswordReset_WorksForAnAddressWithUppercaseNonASCII(t *testing.T) {
	const stored = "JÖRG@example.test"
	for _, typed := range []string{stored, "jörg@example.test", "  Jörg@EXAMPLE.test "} {
		t.Run(typed, func(t *testing.T) {
			h := newEmailKeyHarness(t)
			h.mustRegister(stored)
			if got, body := h.post("/api/v1/user/remind-password", map[string]string{"username": typed}); got != http.StatusOK {
				t.Fatalf("remind = %d: %s", got, body)
			}
			msgs := h.mail.take()
			if len(msgs) != 1 {
				t.Fatalf("mails = %d, want 1", len(msgs))
			}
			if msgs[0].To != stored {
				t.Fatalf("code sent to %q, want the stored %q", msgs[0].To, stored)
			}
			const newPassword = "a-brand-new-password"
			if got, body := h.post("/api/v1/user/reset-password", map[string]string{
				"username": typed, "code": resetCode.FindString(msgs[0].Text), "password": newPassword,
			}); got != http.StatusOK {
				t.Fatalf("reset = %d: %s", got, body)
			}
			if got := h.login(stored, newPassword); got != http.StatusOK {
				t.Fatalf("login with the new password = %d", got)
			}
		})
	}
}

// İ and the Kelvin sign lowercase to ASCII letters but spell a different mailbox.
func TestPasswordReset_ALookAlikeAddressNeverReachesTheAccount(t *testing.T) {
	cases := []struct{ owner, lookAlike string }{
		{"admin@gmail.test", "admin@gmaİl.test"},
		{"admin@gmail.test", "admİn@gmail.test"},
		{"kate@example.test", "Kate@example.test"},
	}
	for _, c := range cases {
		t.Run(c.lookAlike, func(t *testing.T) {
			h := newEmailKeyHarness(t)
			h.mustRegister(c.owner)
			if got, _ := h.post("/api/v1/user/remind-password", map[string]string{"username": c.lookAlike}); got != http.StatusOK {
				t.Fatalf("remind = %d", got)
			}
			if msgs := h.mail.take(); len(msgs) != 0 {
				t.Fatalf("a code went to %q", msgs[0].To)
			}
			if got := h.login(c.lookAlike, emailKeyPassword); got != http.StatusUnauthorized {
				t.Fatalf("login through the look-alike = %d, want 401", got)
			}
		})
	}
}

func TestConfirmEmailChange_CaseOfTheNewAddressStillResolvesTheAccount(t *testing.T) {
	h := newEmailKeyHarness(t)
	h.mustRegister("old@example.test")
	got, body := h.post("/api/v1/user/login-user", map[string]string{"username": "old@example.test", "password": emailKeyPassword})
	if got != http.StatusOK {
		t.Fatalf("login = %d: %s", got, body)
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &login); err != nil {
		t.Fatal(err)
	}
	h.mail.take()
	authed := func(path string, payload map[string]string) int {
		raw, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, h.ts.URL+path, strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+login.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	const moved = "NEW-Ö@example.test"
	if got := authed("/api/v1/user/request-email-change", map[string]string{"newEmail": moved, "password": emailKeyPassword}); got != http.StatusOK {
		t.Fatalf("request change = %d", got)
	}
	var code string
	for _, m := range h.mail.take() {
		if m.To == moved {
			code = resetCode.FindString(m.Text)
		}
	}
	if code == "" {
		t.Fatal("no code mailed to the new address")
	}
	if got := authed("/api/v1/user/confirm-email-change", map[string]string{"code": code}); got != http.StatusOK {
		t.Fatalf("confirm change = %d", got)
	}
	if got := h.login("new-ö@EXAMPLE.test", emailKeyPassword); got != http.StatusOK {
		t.Fatalf("login after the change, other case = %d", got)
	}
	if got, _ := h.register("new-ö@example.test"); got != http.StatusBadRequest {
		t.Fatalf("registering the moved-to address = %d, want 400", got)
	}
	if got := h.login("old@example.test", emailKeyPassword); got != http.StatusUnauthorized {
		t.Fatalf("login with the old address = %d, want 401", got)
	}
	h.mustRegister("old@example.test")
}

// A pair registered before the key existed: the older account holds the key and the
// newer one keeps NULL, the state the migration leaves it in.
func (h emailKeyHarness) seedCollidingPair(holder, other string) {
	h.t.Helper()
	h.mustRegister(holder)
	const placeholder = "placeholder@example.test"
	h.mustRegister(placeholder)
	if _, err := h.db.Raw.Exec(h.db.Rebind(`UPDATE users SET email = ?, email_key = NULL WHERE email = ?`), other, placeholder); err != nil {
		h.t.Fatal(err)
	}
}

func (h emailKeyHarness) signedInID(email, password string) (id, token string) {
	h.t.Helper()
	got, body := h.post("/api/v1/user/login-user", map[string]string{"username": email, "password": password})
	if got != http.StatusOK {
		h.t.Fatalf("login %s = %d: %s", email, got, body)
	}
	var login struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &login); err != nil {
		h.t.Fatal(err)
	}
	return login.User.ID, login.Token
}

func TestCollidingAccounts_EachSpellingKeepsItsOwnAccount(t *testing.T) {
	const holder, other = "jörg@example.test", "JÖRG@example.test"
	h := newEmailKeyHarness(t)
	h.seedCollidingPair(holder, other)

	holderID, _ := h.signedInID(holder, emailKeyPassword)
	otherID, otherToken := h.signedInID(other, emailKeyPassword)
	if holderID == otherID {
		t.Fatal("both spellings resolved to one account")
	}
	if variantID, _ := h.signedInID("JöRG@example.test", emailKeyPassword); variantID != holderID {
		t.Fatal("another spelling must resolve to the account holding the key")
	}

	// A save must not write the key the older account holds onto this row.
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+"/api/v1/user/update-name", strings.NewReader(`{"name":"Renamed"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("saving the colliding account = %d", resp.StatusCode)
	}
	if again, _ := h.signedInID(other, emailKeyPassword); again != otherID {
		t.Fatal("the colliding account lost its spelling after a save")
	}

	for _, email := range []string{holder, other} {
		if got, _ := h.post("/api/v1/user/remind-password", map[string]string{"username": email}); got != http.StatusOK {
			t.Fatalf("remind %s = %d", email, got)
		}
		msgs := h.mail.take()
		if len(msgs) != 1 || msgs[0].To != email {
			t.Fatalf("remind %s mailed %+v", email, msgs)
		}
	}
	if got, _ := h.register("JÖRG@EXAMPLE.test"); got != http.StatusBadRequest {
		t.Fatalf("a third spelling registered with %d, want 400", got)
	}
}

func TestCollidingAccounts_AResetChangesOnlyTheAccountThatProvedItsAddress(t *testing.T) {
	const holder, other, newPassword = "jörg@example.test", "JÖRG@example.test", "a-brand-new-password"
	h := newEmailKeyHarness(t)
	h.seedCollidingPair(holder, other)
	if got, _ := h.post("/api/v1/user/remind-password", map[string]string{"username": other}); got != http.StatusOK {
		t.Fatalf("remind = %d", got)
	}
	msgs := h.mail.take()
	if len(msgs) != 1 {
		t.Fatalf("mails = %d, want 1", len(msgs))
	}
	if got, body := h.post("/api/v1/user/reset-password", map[string]string{
		"username": other, "code": resetCode.FindString(msgs[0].Text), "password": newPassword,
	}); got != http.StatusOK {
		t.Fatalf("reset = %d: %s", got, body)
	}
	if got := h.login(other, newPassword); got != http.StatusOK {
		t.Fatalf("login to the reset account = %d", got)
	}
	if got := h.login(holder, emailKeyPassword); got != http.StatusOK {
		t.Fatalf("the other account's password moved: login = %d", got)
	}
}
