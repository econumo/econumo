package apiparity

// budget_labels_populated pins the get-budget "structure.labels" block
// (internal/budget/builder_structure_build.go) with an ACTUAL populated
// array — every other scenario's golden shows "labels":[], which exercises
// nothing about the labels block itself and, critically, is never
// byte-compared cross-engine by enginecompare for a non-empty case.
//
// One expense carries BOTH LabelWork and LabelPersonal, so the deliberate
// overlap (a label has no limit and takes no part in envelope math; two
// labels on one transaction each show the FULL amount, not a split) is
// directly visible: both entries must read "spent":"50".
//
// LabelWork/LabelPersonal are seeded (fixture.go's Seed) with LabelPersonal's
// higher-sorting id ("...02") given the LOWER position (0) and LabelWork's
// lower-sorting id ("...01") the HIGHER position (1) — so the expected
// [LabelPersonal, LabelWork] order in the golden can only pass if the
// builder truly sorts by position, not by falling back to id order.
//
// The period is a fixed literal date, matching budget_uncategorized_element.go's
// rationale: get-budget's periodStart/End land in the response body and would
// otherwise bake today's date into the committed golden.
func init() {
	const period = "2024-04-01"
	register(Scenario{Name: "budget_labels_populated", Calls: func() []Call {
		return []Call{
			{Label: "create-double-label-expense", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000fe", "accountId": OwnerAccount, "type": "expense",
					"amount": "50.00", "date": period + " 09:00:00",
					"labelIds": []string{LabelWork, LabelPersonal},
				}},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget + "&date=" + period, Auth: "owner"},
		}
	}})
}
