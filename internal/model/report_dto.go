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
