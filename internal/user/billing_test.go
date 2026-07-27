package user

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type stubSigner struct{}

func (stubSigner) Sign(uid string, _ time.Time) (string, error) { return "tok-" + uid, nil }

type billingClock struct{}

func (billingClock) Now() time.Time { return time.Unix(0, 0).UTC() }

func newBillingSvc(base string) *BillingService {
	return NewBillingService(base, stubSigner{}, billingClock{})
}

func TestCreateBillingLinkCarriesTokenAndLang(t *testing.T) {
	svc := newBillingSvc("https://pay.example.test/cloud/")
	ctx := reqctx.WithLanguage(context.Background(), "ru")
	id := vo.NewId()

	res, err := svc.CreateBillingLink(ctx, id, model.CreateBillingLinkRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("t"); got != "tok-"+id.String() {
		t.Fatalf("t = %q", got)
	}
	if got := u.Query().Get("lang"); got != "ru" {
		t.Fatalf("lang = %q, want ru", got)
	}
	if u.Query().Has("for") {
		t.Fatal("for must be absent when not requested")
	}
}

func TestCreateBillingLinkDefaultsLangToEnglish(t *testing.T) {
	res, err := newBillingSvc("https://pay.example.test/cloud/").
		CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if got := u.Query().Get("lang"); got != "en" {
		t.Fatalf("lang = %q, want en", got)
	}
}

func TestCreateBillingLinkPreservesExistingQuery(t *testing.T) {
	res, err := newBillingSvc("https://pay.example.test/cloud/?src=app").
		CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if u.Query().Get("src") != "app" {
		t.Fatalf("existing query lost: %s", res.URL)
	}
}

func TestCreateBillingLinkPassesFor(t *testing.T) {
	partner := vo.NewId()
	res, err := newBillingSvc("https://pay.example.test/cloud/").
		CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{For: partner.String()})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if got := u.Query().Get("for"); got != partner.String() {
		t.Fatalf("for = %q", got)
	}
}

func TestCreateBillingLinkRejectsMalformedFor(t *testing.T) {
	_, err := newBillingSvc("https://pay.example.test/cloud/").
		CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{For: "not-a-uuid&evil=1"})
	if err == nil {
		t.Fatal("malformed for accepted")
	}
}

func TestCreateBillingLinkDisabledWithoutURL(t *testing.T) {
	_, err := newBillingSvc("").
		CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{})
	if err == nil {
		t.Fatal("want an error when ECONUMO_BILLING_URL is unset")
	}
}

func TestCreateBillingLinkLogsBeneficiary(t *testing.T) {
	svc := newBillingSvc("https://pay.example.test/cloud/")
	ctx := reqctx.WithLogAttrs(context.Background())
	partner := vo.NewId()
	if _, err := svc.CreateBillingLink(ctx, vo.NewId(),
		model.CreateBillingLinkRequest{For: partner.String()}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range reqctx.LogAttrs(ctx) {
		if a.Key == "for_user_id" && a.Value.String() == partner.String() {
			found = true
		}
	}
	if !found {
		t.Fatal("for_user_id attr missing — minting a link for a partner must be auditable")
	}
}
