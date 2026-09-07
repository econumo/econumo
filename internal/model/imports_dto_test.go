package model_test

import (
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	out := map[string]string{}
	for _, f := range verr.Fields {
		out[f.Key] = f.Code
	}
	return out
}

func TestCreateImportSourceRequest_Validate(t *testing.T) {
	if err := (model.CreateImportSourceRequest{Provider: "apple-wallet", Name: "iPhone"}).Validate(); err != nil {
		t.Fatalf("apple-wallet needs no ciphertext: %v", err)
	}
	if err := (model.CreateImportSourceRequest{Provider: "simplefin", Name: "Bank", CredentialCiphertext: "v1:a:b"}).Validate(); err != nil {
		t.Fatalf("simplefin with ciphertext: %v", err)
	}
	codes := fieldCodes(t, model.CreateImportSourceRequest{Provider: "simplefin", Name: "Bank"}.Validate())
	if codes["credentialCiphertext"] != errs.CodeIsBlank {
		t.Fatalf("simplefin without ciphertext: %v", codes)
	}
	codes = fieldCodes(t, model.CreateImportSourceRequest{Provider: "csv", Name: "x"}.Validate())
	if codes["provider"] != errs.CodeImportProviderUnsupported {
		t.Fatalf("csv: %v", codes)
	}
}

func TestSyncImportSourceRequest_Validate(t *testing.T) {
	ok := model.SyncImportSourceRequest{SourceId: "s", AccessUrl: "https://u:p@bridge.example/x", StartDate: "2026-08-01"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("endDate is optional: %v", err)
	}
	codes := fieldCodes(t, model.SyncImportSourceRequest{SourceId: "s", AccessUrl: "u", StartDate: "08/01/2026", EndDate: "nope"}.Validate())
	if codes["startDate"] != errs.CodeInvalidDatetime || codes["endDate"] != errs.CodeInvalidDatetime {
		t.Fatalf("bad dates: %v", codes)
	}
	codes = fieldCodes(t, model.SyncImportSourceRequest{}.Validate())
	for _, k := range []string{"sourceId", "accessUrl", "startDate"} {
		if codes[k] != errs.CodeIsBlank {
			t.Fatalf("%s should be blank-checked: %v", k, codes)
		}
	}
}

func TestSetImportCredentialKeyRequest_Validate(t *testing.T) {
	long := make([]byte, 4097)
	for i := range long {
		long[i] = 'a'
	}
	codes := fieldCodes(t, model.SetImportCredentialKeyRequest{WrappedDataKey: string(long), Kdf: ""}.Validate())
	if codes["wrappedDataKey"] != errs.CodeTooLong || codes["kdf"] != errs.CodeIsBlank {
		t.Fatalf("codes: %v", codes)
	}
}

func TestClaimAndListRequests_Validate(t *testing.T) {
	if codes := fieldCodes(t, model.ClaimSetupTokenRequest{}.Validate()); codes["setupToken"] != errs.CodeIsBlank {
		t.Fatal(codes)
	}
	if codes := fieldCodes(t, model.ListExternalAccountsRequest{SourceId: "s"}.Validate()); codes["accessUrl"] != errs.CodeIsBlank {
		t.Fatal(codes)
	}
}
