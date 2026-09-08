package imports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestClaimSetupToken(t *testing.T) {
	h, p := bankHarness(t)
	res, err := h.svc.ClaimSetupToken(context.Background(), vo.MustParseId(userA), model.ClaimSetupTokenRequest{SetupToken: "abc"})
	if err != nil || res.AccessUrl != "https://u:p@bridge.example/simplefin" || p.claimed != "abc" {
		t.Fatalf("res = %+v err %v", res, err)
	}
	_, err = h.svc.ClaimSetupToken(context.Background(), vo.MustParseId(userA), model.ClaimSetupTokenRequest{SetupToken: "used"})
	var denied *errs.AccessDeniedError
	if !errorsAs(err, &denied) || denied.Code != errs.CodeImportSetupTokenRejected {
		t.Fatalf("err = %v", err)
	}
}

func TestClaimSetupToken_NoClaimer(t *testing.T) {
	h := setup(t) // no provider registered
	_, err := h.svc.ClaimSetupToken(context.Background(), vo.MustParseId(userA), model.ClaimSetupTokenRequest{SetupToken: "abc"})
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportProviderUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func TestCredentialKey_RoundTrip(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	if _, err := h.svc.GetCredentialKey(ctx, vo.MustParseId(userA)); err == nil {
		t.Fatal("expected not found before set")
	}
	res, err := h.svc.SetCredentialKey(ctx, vo.MustParseId(userA), model.SetImportCredentialKeyRequest{WrappedDataKey: "v1:iv:ct", Kdf: `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`})
	if err != nil || res.WrappedDataKey != "v1:iv:ct" || res.UpdatedAt == "" {
		t.Fatalf("set = %+v err %v", res, err)
	}
	res, err = h.svc.SetCredentialKey(ctx, vo.MustParseId(userA), model.SetImportCredentialKeyRequest{WrappedDataKey: "v1:iv2:ct2", Kdf: res.Kdf})
	if err != nil || res.WrappedDataKey != "v1:iv2:ct2" {
		t.Fatalf("overwrite = %+v err %v", res, err)
	}
	got, err := h.svc.GetCredentialKey(ctx, vo.MustParseId(userA))
	if err != nil || got.WrappedDataKey != "v1:iv2:ct2" {
		t.Fatalf("get = %+v err %v", got, err)
	}
	if _, err := h.svc.GetCredentialKey(ctx, vo.MustParseId(userB)); err == nil {
		t.Fatal("keys are per user")
	}
}

func TestListExternalAccounts_MergesMappingState(t *testing.T) {
	h, p := bankHarness(t)
	p.accounts = append(p.accounts, model.ExternalAccount{ID: "ACT-2", Name: "Savings", Currency: "USD", Balance: "5"})
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-2", AccountID: acct1})
	res, err := h.svc.ListExternalAccounts(context.Background(), vo.MustParseId(userA), model.ListExternalAccountsRequest{SourceId: bankSource, AccessUrl: "https://u:p@bridge.example/simplefin"})
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("res = %+v err %v", res, err)
	}
	if res.Items[0].ExternalAccountId != "ACT-1" || res.Items[0].State != model.ImportCardStateUnmapped || res.Items[0].Balance != "10" {
		t.Fatalf("item0 = %+v", res.Items[0])
	}
	if res.Items[1].State != model.ImportCardStateMapped || res.Items[1].AccountId != acct1 {
		t.Fatalf("item1 = %+v", res.Items[1])
	}
	if len(p.seen) != 1 || p.seen[0].AccessURL != "https://u:p@bridge.example/simplefin" {
		t.Fatalf("provider saw %+v", p.seen)
	}
	_, err = h.svc.ListExternalAccounts(context.Background(), vo.MustParseId(userA), model.ListExternalAccountsRequest{SourceId: source, AccessUrl: "x"})
	if verr, ok := errs.AsValidation(err); !ok || verr.MsgCode != errs.CodeImportProviderUnsupported {
		t.Fatalf("push provider: %v", err)
	}
}

func TestCreateSource_SimpleFINStoresAndReconnectOverwritesCiphertext(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	res, err := h.svc.CreateSource(ctx, vo.MustParseId(userA), model.CreateImportSourceRequest{Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:a:b"})
	if err != nil || res.Item.CredentialCiphertext != "v1:a:b" || res.Item.LastSyncedAt != "" {
		t.Fatalf("res = %+v err %v", res, err)
	}
	again, err := h.svc.CreateSource(ctx, vo.MustParseId(userA), model.CreateImportSourceRequest{Provider: model.ImportProviderSimpleFIN, Name: "Bank 2", CredentialCiphertext: "v1:c:d"})
	if err != nil || again.Item.Id != res.Item.Id || again.Item.CredentialCiphertext != "v1:c:d" || again.Item.Name != "Bank 2" {
		t.Fatalf("reconnect = %+v err %v", again, err)
	}
	// Apple Wallet keeps its ciphertext empty and its result carries ""
	wallet, err := h.svc.CreateSource(ctx, vo.MustParseId(userA), model.CreateImportSourceRequest{Provider: model.ImportProviderAppleWallet, Name: "iPhone"})
	if err != nil || wallet.Item.CredentialCiphertext != "" {
		t.Fatalf("wallet = %+v err %v", wallet, err)
	}
}

func errorsAs(err error, target any) bool { return err != nil && errors.As(err, target) }
