package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
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

func TestCreateImportSourceRequest_CiphertextShape(t *testing.T) {
	long := "v1:" + strings.Repeat("a", 4094)
	codes := fieldCodes(t, model.CreateImportSourceRequest{Provider: "simplefin", Name: "Bank", CredentialCiphertext: long}.Validate())
	if codes["credentialCiphertext"] != errs.CodeTooLong {
		t.Fatalf("over the cap: %v", codes)
	}
	// A plaintext access URL sent here is a client bug, and persisting it would
	// hand the server the secret the design keeps off it.
	codes = fieldCodes(t, model.CreateImportSourceRequest{Provider: "simplefin", Name: "Bank", CredentialCiphertext: "https://u:p@bridge.example/simplefin"}.Validate())
	if codes["credentialCiphertext"] != errs.CodeInvalidFormat {
		t.Fatalf("unversioned ciphertext: %v", codes)
	}
}

func TestImportAccountRequests_ExternalNameLength(t *testing.T) {
	long := strings.Repeat("n", 256)
	codes := fieldCodes(t, model.LinkImportAccountRequest{SourceId: "s", ExternalAccountId: "a", AccountId: "b", ExternalName: long}.Validate())
	if codes["externalName"] != errs.CodeTooLong {
		t.Fatalf("link: %v", codes)
	}
	codes = fieldCodes(t, model.ImportAccountActionRequest{SourceId: "s", ExternalAccountId: "a", ExternalName: long}.Validate())
	if codes["externalName"] != errs.CodeTooLong {
		t.Fatalf("action: %v", codes)
	}
	if err := (model.ImportAccountActionRequest{SourceId: "s", ExternalAccountId: "a", ExternalName: long[:255]}).Validate(); err != nil {
		t.Fatalf("255 chars is allowed: %v", err)
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

func classifySpec() model.ImportRuleSpec {
	cat := "0192d6d4-0000-7000-8000-000000000001"
	return model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee,
		MatchType: model.ImportRuleMatchTypeContains, MatchValue: "STARBUCKS", CategoryId: &cat,
	}
}

func TestImportRuleSpec_Validate(t *testing.T) {
	if err := classifySpec().Validate(); err != nil {
		t.Fatalf("valid classify spec: %v", err)
	}
	skip := model.ImportRuleSpec{Action: model.ImportRuleActionSkip, MatchField: model.ImportRuleMatchFieldDescription, MatchType: model.ImportRuleMatchTypePrefix, MatchValue: "PAYMENT - THANK YOU"}
	if err := skip.Validate(); err != nil {
		t.Fatalf("valid skip spec: %v", err)
	}

	blank := classifySpec()
	blank.MatchValue = "   "
	if got := fieldCodes(t, blank.Validate()); got["matchValue"] != errs.CodeIsBlank {
		t.Errorf("blank matchValue: %v", got)
	}
	long := classifySpec()
	long.MatchValue = strings.Repeat("x", 256)
	if got := fieldCodes(t, long.Validate()); got["matchValue"] != errs.CodeTooLong {
		t.Errorf("long matchValue: %v", got)
	}
	for _, tc := range []struct {
		name  string
		mut   func(*model.ImportRuleSpec)
		field string
	}{
		{"bad action", func(s *model.ImportRuleSpec) { s.Action = "delete" }, "action"},
		{"reserved match field", func(s *model.ImportRuleSpec) { s.MatchField = "external_category" }, "matchField"},
		{"bad match type", func(s *model.ImportRuleSpec) { s.MatchType = "regex" }, "matchType"},
		{"bad source id", func(s *model.ImportRuleSpec) { v := "nope"; s.SourceId = &v }, "sourceId"},
		{"bad category id", func(s *model.ImportRuleSpec) { v := "nope"; s.CategoryId = &v }, "categoryId"},
		{"bad label id", func(s *model.ImportRuleSpec) { s.LabelIds = []string{"nope"} }, "labelIds"},
	} {
		s := classifySpec()
		tc.mut(&s)
		got := fieldCodes(t, s.Validate())
		if tc.field == "action" || tc.field == "matchField" || tc.field == "matchType" {
			if got[tc.field] != errs.CodeInvalidChoice {
				t.Errorf("%s: %v", tc.name, got)
			}
		} else if got[tc.field] != errs.CodeInvalidFormat {
			t.Errorf("%s: %v", tc.name, got)
		}
	}

	// classify with nothing to apply is pointless; skip with targets/labels is contradictory
	empty := classifySpec()
	empty.CategoryId = nil
	if got := fieldCodes(t, empty.Validate()); got["action"] != errs.CodeInvalidChoice {
		t.Errorf("classify without targets: %v", got)
	}
	cat := "0192d6d4-0000-7000-8000-000000000001"
	skipWithTarget := skip
	skipWithTarget.CategoryId = &cat
	if got := fieldCodes(t, skipWithTarget.Validate()); got["categoryId"] != errs.CodeInvalidChoice {
		t.Errorf("skip with target: %v", got)
	}
	skipWithLabels := skip
	skipWithLabels.LabelIds = []string{"0192d6d4-0000-7000-8000-000000000002"}
	if got := fieldCodes(t, skipWithLabels.Validate()); got["labelIds"] != errs.CodeInvalidChoice {
		t.Errorf("skip with labels: %v", got)
	}
	tooMany := classifySpec()
	for i := 0; i < model.MaxImportRuleLabels+1; i++ {
		tooMany.LabelIds = append(tooMany.LabelIds, vo.NewId().String())
	}
	if got := fieldCodes(t, tooMany.Validate()); got["labelIds"] != errs.CodeTooLong {
		t.Errorf("too many labels: %v", got)
	}
}

func TestPreviewAndApplyImportRuleRequest_Validate(t *testing.T) {
	p := model.PreviewImportRuleRequest{ImportRuleSpec: classifySpec(), Scope: "run"}
	if got := fieldCodes(t, p.Validate()); got["runId"] != errs.CodeIsBlank {
		t.Errorf("scope=run without runId: %v", got)
	}
	p.Scope = "source"
	if got := fieldCodes(t, p.Validate()); got["scopeSourceId"] != errs.CodeIsBlank {
		t.Errorf("scope=source without scopeSourceId: %v", got)
	}
	p.Scope = "everything"
	if got := fieldCodes(t, p.Validate()); got["scope"] != errs.CodeInvalidChoice {
		t.Errorf("bad scope: %v", got)
	}
	p.Scope = "all"
	if err := p.Validate(); err != nil {
		t.Errorf("scope=all: %v", err)
	}
	a := model.ApplyImportRuleRequest{Scope: "all"}
	if got := fieldCodes(t, a.Validate()); got["ruleId"] != errs.CodeIsBlank {
		t.Errorf("apply without ruleId: %v", got)
	}
	a.RuleId = "0192d6d4-0000-7000-8000-000000000009"
	if err := a.Validate(); err != nil {
		t.Errorf("valid apply: %v", err)
	}
	if got := fieldCodes(t, model.UpdateImportRuleRequest{ImportRuleSpec: classifySpec()}.Validate()); got["id"] != errs.CodeIsBlank {
		t.Errorf("update without id: %v", got)
	}
	if got := fieldCodes(t, model.DeleteImportRuleRequest{}.Validate()); got["id"] != errs.CodeIsBlank {
		t.Errorf("delete without id: %v", got)
	}
}
