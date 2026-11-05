package model

type GetNetWorthReportRequest struct {
	BudgetId string `json:"budgetId"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// NetWorthAccountRow is one account's balance at the END of each month,
// converted to the report currency with that month's average rate.
type NetWorthAccountRow struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Icon        string   `json:"icon"`
	CurrencyId  string   `json:"currencyId"`
	OwnerUserId string   `json:"ownerUserId"`
	IsSavings   int      `json:"isSavings"`
	Balances    []string `json:"balances"`
}

type NetWorthReportResult struct {
	CurrencyId string               `json:"currencyId"`
	Months     []string             `json:"months"`
	Accounts   []NetWorthAccountRow `json:"accounts"`
}

type GetNetWorthReportResult struct {
	Item NetWorthReportResult `json:"item"`
}
