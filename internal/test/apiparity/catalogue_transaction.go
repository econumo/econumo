package apiparity

// Transaction-module scenarios for update, CSV export, and multipart CSV
// import — the 3 routes carved out of missingFromCatalogue by this file.

import (
	"bytes"
	"mime/multipart"
)

func init() {
	register(Scenario{Name: "transaction_writes", Calls: func() []Call {
		return []Call{
			{Label: "update-transaction", Method: "POST", Path: "/api/v1/transaction/update-transaction", Auth: "owner",
				Body: map[string]any{
					"id": Txn1, "type": "expense", "amount": "15.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"date":        ClockTime.Format("2006-01-02 15:04:05"),
					"description": "lunch updated", "payeeId": PayeeShop,
				}},
			{Label: "export-transaction-list", Method: "GET", Path: "/api/v1/transaction/export-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})

	register(Scenario{Name: "transaction_import", Calls: func() []Call {
		body, ctype := buildImportBody()
		return []Call{
			{Label: "import-transaction-list", Method: "POST", Path: "/api/v1/transaction/import-transaction-list",
				Auth: "owner", RawBody: body, ContentType: ctype},
			{Label: "get-transaction-list-after", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})
}

// buildImportBody assembles the multipart form the import endpoint expects: a
// CSV file, a column-mapping JSON, and a fixed accountId override (so the
// mapping does not need an "account" column). The CSV amount is positive
// (income) and the category name matches the seeded CatFood so the row
// resolves to an existing category rather than creating a new one.
func buildImportBody() ([]byte, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "import.csv")
	if err != nil {
		panic(err)
	}
	if _, err := fw.Write([]byte("Date,Amount,Description,Category\n2026-01-15,12.34,coffee,Food\n")); err != nil {
		panic(err)
	}
	if err := w.WriteField("mapping", `{"date":"Date","amount":"Amount","description":"Description","category":"Category"}`); err != nil {
		panic(err)
	}
	if err := w.WriteField("accountId", OwnerAccount); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes(), w.FormDataContentType()
}
