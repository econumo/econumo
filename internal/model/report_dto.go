package model

type GetFlowReportRequest struct {
	BudgetId string `json:"budgetId"`
	From     string `json:"from"`
	To       string `json:"to"`
	GroupBy  string `json:"groupBy"`
}

// ReportRow is one expense/income/savings row of the flow report. Amounts[i]
// is in the report currency for Months[i]; Planned has the same length in
// budget scope ("" = no plan) and is empty in the my-accounts scope.
type ReportRow struct {
	Id          string           `json:"id"`
	Kind        string           `json:"kind"`
	Name        string           `json:"name"`
	Icon        string           `json:"icon"`
	IsArchived  int              `json:"isArchived"`
	OwnerUserId *string          `json:"ownerUserId"`
	Amounts     []string         `json:"amounts"`
	Planned     []string         `json:"planned"`
	Children    []ReportChildRow `json:"children"`
}

type ReportChildRow struct {
	Id         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Icon       string   `json:"icon"`
	IsArchived int      `json:"isArchived"`
	Amounts    []string `json:"amounts"`
}

type FlowReportResult struct {
	CurrencyId   string      `json:"currencyId"`
	Months       []string    `json:"months"`
	Expense      []ReportRow `json:"expense"`
	Income       []ReportRow `json:"income"`
	TransfersIn  []string    `json:"transfersIn"`
	TransfersOut []string    `json:"transfersOut"`
	Savings      []ReportRow `json:"savings"`
}

type GetFlowReportResult struct {
	Item FlowReportResult `json:"item"`
}

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
	IsDeleted   int      `json:"isDeleted"`
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
