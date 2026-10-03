package apiparity

import (
	"context"
	"encoding/json"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
)

// stubProvider is the pull provider the parity harness wires in place of the
// real SimpleFIN client: deterministic, offline, and keyed on the access URL
// so one scenario can exercise both the healthy and the failing bridge.
//
//   - access URL "https://u:p@bridge.example/simplefin"  -> two accounts, three rows
//   - access URL "https://u:p@bridge.example/down"       -> ErrProviderUnavailable
//   - anything else                                       -> ErrCredentialInvalid
//   - setup token "used"                                  -> ErrSetupTokenRejected
type stubProvider struct{}

const (
	stubAccessURL = "https://u:p@bridge.example/simplefin"
	stubDownURL   = "https://u:p@bridge.example/down"
)

func stubAccounts() []model.ExternalAccount {
	return []model.ExternalAccount{
		{ID: "ACT-CHK", Name: "Checking", Currency: "USD", Balance: "1250.10", OrgName: "Example Bank"},
		{ID: "ACT-SAV", Name: "Savings", Currency: "USD", Balance: "9000", OrgName: "Example Bank"},
	}
}

func stubRow(account, id, amount string, posted int64, payee string) model.ExternalTransaction {
	raw, _ := json.Marshal(map[string]any{"id": id, "posted": posted, "amount": amount, "payee": payee, "description": payee, "pending": false})
	return model.ExternalTransaction{ExternalAccountID: account, ID: id, Amount: amount, Posted: posted, Payee: payee, Description: payee, Raw: raw}
}

func (stubProvider) ListAccounts(_ context.Context, cred imports.Credential) ([]model.ExternalAccount, error) {
	switch cred.AccessURL {
	case stubAccessURL:
		return stubAccounts(), nil
	case stubDownURL:
		return nil, imports.ErrProviderUnavailable
	}
	return nil, imports.ErrCredentialInvalid
}

func (stubProvider) FetchTransactions(_ context.Context, cred imports.Credential, _ imports.FetchRequest) (*imports.FetchResult, error) {
	switch cred.AccessURL {
	case stubAccessURL:
		// ClockTime is time.Now() (harness.go), so posted dates are relative to
		// it: inside the scenario's [ClockTime-30d, ClockTime] range.
		return &imports.FetchResult{
			Accounts: stubAccounts(),
			Transactions: []model.ExternalTransaction{
				stubRow("ACT-CHK", "chk-1", "-12.50", ClockTime.AddDate(0, 0, -3).Unix(), "Blue Bottle"),
				stubRow("ACT-CHK", "chk-2", "2500", ClockTime.AddDate(0, 0, -2).Unix(), "Payroll"),
				stubRow("ACT-SAV", "sav-1", "-100", ClockTime.AddDate(0, 0, -3).Unix(), "Transfer"),
			},
			Warnings: []string{"Connection to Example Bank may need attention"},
		}, nil
	case stubDownURL:
		return nil, imports.ErrProviderUnavailable
	}
	return nil, imports.ErrCredentialInvalid
}

func (stubProvider) ClaimSetupToken(_ context.Context, token string) (string, error) {
	if token == "used" {
		return "", imports.ErrSetupTokenRejected
	}
	return stubAccessURL, nil
}
