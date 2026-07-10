package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	v, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return v
}

var (
	tu0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	tu1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	tu2 = time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
)

// seqIDs returns a NextIdentity-style generator over a fixed list of uuids.
func seqIDs(t *testing.T, uuids ...string) func() vo.Id {
	t.Helper()
	i := 0
	return func() vo.Id {
		if i >= len(uuids) {
			t.Fatalf("ran out of seeded ids (asked for #%d)", i+1)
		}
		id := mustID(t, uuids[i])
		i++
		return id
	}
}

func newUser(t *testing.T) *User {
	return NewUser(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		"identifier-md5", "encrypted-email", "Alice", "https://avatar/x",
		"password-hash", "salt-hex", tu0)
}

// seeded builds a user with the four default options seeded.
func seeded(t *testing.T) *User {
	u := newUser(t)
	u.SeedDefaultOptions(seqIDs(t,
		"aaaaaaaa-0000-0000-0000-000000000001",
		"aaaaaaaa-0000-0000-0000-000000000002",
		"aaaaaaaa-0000-0000-0000-000000000003",
		"aaaaaaaa-0000-0000-0000-000000000004",
	), tu0)
	return u
}

func TestNewUser_Getters(t *testing.T) {
	u := newUser(t)
	if u.Identifier != "identifier-md5" {
		t.Errorf("Identifier()=%q", u.Identifier)
	}
	if u.Email != "encrypted-email" {
		t.Errorf("Email()=%q", u.Email)
	}
	if u.Name != "Alice" {
		t.Errorf("Name()=%q", u.Name)
	}
	if u.Avatar != "https://avatar/x" {
		t.Errorf("Avatar()=%q", u.Avatar)
	}
	if u.Password != "password-hash" {
		t.Errorf("Password()=%q", u.Password)
	}
	if u.Salt != "salt-hex" {
		t.Errorf("Salt()=%q", u.Salt)
	}
	if !u.IsActive {
		t.Error("new user must be active")
	}
	if !u.CreatedAt.Equal(tu0) || !u.UpdatedAt.Equal(tu0) {
		t.Errorf("timestamps: %v / %v", u.CreatedAt, u.UpdatedAt)
	}
}

func TestUser_StructLiteral_RoundTrip(t *testing.T) {
	id := mustID(t, "11111111-1111-1111-1111-111111111111")
	opt := NewUserOption(mustID(t, "aaaaaaaa-0000-0000-0000-000000000001"), OptionBudget, strPtr("b-1"), tu0)
	u := &User{ID: id, Identifier: "ident", Email: "email", Name: "Bob", Avatar: "avatar", Password: "pw",
		Salt: "salt", IsActive: false, CreatedAt: tu0, UpdatedAt: tu1, Options: []UserOption{opt}}
	if !u.ID.Equal(id) || u.Name != "Bob" || u.IsActive {
		t.Fatal("scalar fields did not round-trip")
	}
	if !u.CreatedAt.Equal(tu0) || !u.UpdatedAt.Equal(tu1) {
		t.Errorf("timestamps: %v / %v", u.CreatedAt, u.UpdatedAt)
	}
	if len(u.Options) != 1 {
		t.Fatalf("options len=%d want 1", len(u.Options))
	}
}

func TestUser_DefaultsWhenNoOptions(t *testing.T) {
	u := newUser(t) // no options seeded
	if u.CurrencyCode() != DefaultCurrency {
		t.Errorf("CurrencyCode()=%q want %q", u.CurrencyCode(), DefaultCurrency)
	}
	if u.ReportPeriod() != DefaultReportPeriod {
		t.Errorf("ReportPeriod()=%q want %q", u.ReportPeriod(), DefaultReportPeriod)
	}
	if u.Option(OptionCurrency) != nil {
		t.Error("Option on a user with no options should be nil")
	}
}

func TestUser_SeedDefaultOptions(t *testing.T) {
	u := seeded(t)
	opts := u.Options
	if len(opts) != 4 {
		t.Fatalf("seeded option count=%d want 4", len(opts))
	}
	// Order must match persistedOptions / the registration seed order.
	wantNames := []string{OptionCurrency, OptionReportPeriod, OptionBudget, OptionOnboarding}
	for i, n := range wantNames {
		if opts[i].Name != n {
			t.Errorf("option[%d].Name()=%q want %q", i, opts[i].Name, n)
		}
		if !opts[i].CreatedAt.Equal(tu0) {
			t.Errorf("option[%d] createdAt=%v want %v", i, opts[i].CreatedAt, tu0)
		}
	}
	// Default values.
	if c := u.Option(OptionCurrency); c == nil || c.Value == nil || *c.Value != DefaultCurrency {
		t.Errorf("currency default wrong: %+v", c)
	}
	if rp := u.Option(OptionReportPeriod); rp == nil || rp.Value == nil || *rp.Value != DefaultReportPeriod {
		t.Errorf("report_period default wrong: %+v", rp)
	}
	if b := u.Option(OptionBudget); b == nil || b.Value != nil {
		t.Errorf("budget default should be nil value: %+v", b)
	}
	if ob := u.Option(OptionOnboarding); ob == nil || ob.Value == nil || *ob.Value != OnboardingStarted {
		t.Errorf("onboarding default wrong: %+v", ob)
	}
	// Surfaced helpers reflect the seeded values.
	if u.CurrencyCode() != DefaultCurrency {
		t.Errorf("CurrencyCode()=%q", u.CurrencyCode())
	}
	if u.ReportPeriod() != DefaultReportPeriod {
		t.Errorf("ReportPeriod()=%q", u.ReportPeriod())
	}
}

func TestUser_SeedDefaultOptions_Idempotent(t *testing.T) {
	u := seeded(t)
	// Re-seed: must reset to exactly four (truncate-then-append), not duplicate.
	u.SeedDefaultOptions(seqIDs(t,
		"bbbbbbbb-0000-0000-0000-000000000001",
		"bbbbbbbb-0000-0000-0000-000000000002",
		"bbbbbbbb-0000-0000-0000-000000000003",
		"bbbbbbbb-0000-0000-0000-000000000004",
	), tu1)
	if len(u.Options) != 4 {
		t.Fatalf("re-seed option count=%d want 4", len(u.Options))
	}
}

func TestUser_UpdateName(t *testing.T) {
	u := newUser(t)
	u.UpdateName("Carol", tu1)
	if u.Name != "Carol" || !u.UpdatedAt.Equal(tu1) {
		t.Fatalf("UpdateName: %q / %v", u.Name, u.UpdatedAt)
	}
}

func TestUser_UpdatePassword(t *testing.T) {
	u := newUser(t)
	u.UpdatePassword("new-hash", tu1)
	if u.Password != "new-hash" || !u.UpdatedAt.Equal(tu1) {
		t.Fatalf("UpdatePassword: %q / %v", u.Password, u.UpdatedAt)
	}
}

func TestUser_UpdateEmail(t *testing.T) {
	u := newUser(t)
	u.UpdateEmail("new-ident", "new-cipher", "new-avatar", tu1)
	if u.Identifier != "new-ident" || u.Email != "new-cipher" || u.Avatar != "new-avatar" {
		t.Fatalf("UpdateEmail fields: %q / %q / %q", u.Identifier, u.Email, u.Avatar)
	}
	if !u.UpdatedAt.Equal(tu1) {
		t.Errorf("UpdateEmail updatedAt=%v want %v", u.UpdatedAt, tu1)
	}
}

func TestUser_UpdateCurrency(t *testing.T) {
	u := seeded(t)
	u.UpdateCurrency("EUR", tu1)
	if u.CurrencyCode() != "EUR" {
		t.Fatalf("CurrencyCode()=%q want EUR", u.CurrencyCode())
	}
	o := u.Option(OptionCurrency)
	if !o.UpdatedAt.Equal(tu1) {
		t.Errorf("currency option updatedAt=%v want %v", o.UpdatedAt, tu1)
	}
	// Setting the same value again must not bump the option's updatedAt.
	u.UpdateCurrency("EUR", tu2)
	if !u.Option(OptionCurrency).UpdatedAt.Equal(tu1) {
		t.Error("setting same currency bumped option updatedAt")
	}
}

func TestUser_UpdateCurrency_NoOptionIsNoop(t *testing.T) {
	u := newUser(t) // no options
	u.UpdateCurrency("EUR", tu1)
	if u.Option(OptionCurrency) != nil {
		t.Error("UpdateCurrency must not create an option when none exists")
	}
	if u.CurrencyCode() != DefaultCurrency {
		t.Errorf("CurrencyCode()=%q want default", u.CurrencyCode())
	}
}

func TestUser_UpdateBudget(t *testing.T) {
	u := seeded(t)
	u.UpdateBudget("budget-id-123", tu1)
	o := u.Option(OptionBudget)
	if o == nil || o.Value == nil || *o.Value != "budget-id-123" {
		t.Fatalf("budget option: %+v", o)
	}
	if !o.UpdatedAt.Equal(tu1) {
		t.Errorf("budget option updatedAt=%v want %v", o.UpdatedAt, tu1)
	}
}

func TestUser_CompleteOnboarding(t *testing.T) {
	u := seeded(t)
	u.CompleteOnboarding(tu1)
	o := u.Option(OptionOnboarding)
	if o == nil || o.Value == nil || *o.Value != OnboardingCompleted {
		t.Fatalf("onboarding option: %+v", o)
	}
	if !o.UpdatedAt.Equal(tu1) {
		t.Errorf("onboarding option updatedAt=%v want %v", o.UpdatedAt, tu1)
	}
	// Re-completing is a no-op for updatedAt.
	u.CompleteOnboarding(tu2)
	if !u.Option(OptionOnboarding).UpdatedAt.Equal(tu1) {
		t.Error("re-completing onboarding bumped updatedAt")
	}
}

// TestUser_UpdateReportPeriod writes the period into the report_period option
// and leaves the currency option untouched.
func TestUser_UpdateReportPeriod(t *testing.T) {
	u := seeded(t)
	u.UpdateReportPeriod("weekly", tu1)

	// The report_period option holds the new value.
	rp := u.Option(OptionReportPeriod)
	if rp == nil || rp.Value == nil || *rp.Value != "weekly" {
		t.Fatalf("expected report_period option 'weekly', got %+v", rp)
	}
	// The currency option is untouched at its seeded default.
	cur := u.Option(OptionCurrency)
	if cur == nil || cur.Value == nil || *cur.Value != DefaultCurrency {
		t.Fatalf("currency option should be untouched at %q, got %+v", DefaultCurrency, cur)
	}
	// ReportPeriod() reads the updated report_period option.
	if u.ReportPeriod() != "weekly" {
		t.Errorf("ReportPeriod()=%q want %q", u.ReportPeriod(), "weekly")
	}
}

func TestUserOption_Accessors_AndReconstitute(t *testing.T) {
	id := mustID(t, "aaaaaaaa-0000-0000-0000-000000000001")
	o := ReconstituteUserOption(id, OptionBudget, strPtr("b-9"), tu0, tu1)
	if !o.ID.Equal(id) {
		t.Errorf("Id()=%v want %v", o.ID, id)
	}
	if o.Name != OptionBudget {
		t.Errorf("Name()=%q", o.Name)
	}
	if o.Value == nil || *o.Value != "b-9" {
		t.Errorf("Value()=%v want b-9", o.Value)
	}
	if !o.CreatedAt.Equal(tu0) || !o.UpdatedAt.Equal(tu1) {
		t.Errorf("timestamps: %v / %v", o.CreatedAt, o.UpdatedAt)
	}
}

func TestUserOption_setValue_NilTransitions(t *testing.T) {
	now := tu0
	// nil -> nil is a no-op (no bump).
	o := NewUserOption(mustID(t, "aaaaaaaa-0000-0000-0000-000000000001"), OptionBudget, nil, now)
	o.setValue(nil, tu1)
	if !o.UpdatedAt.Equal(tu0) {
		t.Error("nil->nil bumped updatedAt")
	}
	// nil -> value bumps.
	o.setValue(strPtr("x"), tu1)
	if o.Value == nil || *o.Value != "x" || !o.UpdatedAt.Equal(tu1) {
		t.Fatalf("nil->value: %v / %v", o.Value, o.UpdatedAt)
	}
	// same value -> no bump.
	o.setValue(strPtr("x"), tu2)
	if !o.UpdatedAt.Equal(tu1) {
		t.Error("same value bumped updatedAt")
	}
	// value -> nil bumps.
	o.setValue(nil, tu2)
	if o.Value != nil || !o.UpdatedAt.Equal(tu2) {
		t.Fatalf("value->nil: %v / %v", o.Value, o.UpdatedAt)
	}
}

func TestEqualStrPtr(t *testing.T) {
	a, b := "x", "x"
	c := "y"
	cases := []struct {
		name string
		l, r *string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"left nil", nil, &a, false},
		{"right nil", &a, nil, false},
		{"equal", &a, &b, true},
		{"differ", &a, &c, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := equalStrPtr(tc.l, tc.r); got != tc.want {
				t.Errorf("equalStrPtr=%v want %v", got, tc.want)
			}
		})
	}
}

func TestOptionConstants_Pinned(t *testing.T) {
	// These names + defaults are part of the wire/storage contract.
	pins := map[string]string{
		OptionCurrency:      "currency",
		OptionCurrencyID:    "currency_id",
		OptionReportPeriod:  "report_period",
		OptionBudget:        "budget",
		OptionOnboarding:    "onboarding",
		DefaultCurrency:     "USD",
		DefaultReportPeriod: "monthly",
		OnboardingStarted:   "started",
		OnboardingCompleted: "completed",
	}
	for got, want := range pins {
		if got != want {
			t.Errorf("constant drifted: got %q want %q", got, want)
		}
	}
}
