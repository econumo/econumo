// Package mcp is the currency feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type currenciesDoc struct {
	Currencies []model.CurrencyResult     `json:"currencies"`
	Rates      []model.CurrencyRateResult `json:"rates"`
}

type emptyInput struct{}

func Register(read *appcurrency.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		load := func(ctx context.Context, userID vo.Id) (currenciesDoc, error) {
			list, err := read.GetCurrencyList(ctx, userID)
			if err != nil {
				return currenciesDoc{}, err
			}
			rates, err := read.GetCurrencyRateList(ctx, userID)
			if err != nil {
				return currenciesDoc{}, err
			}
			return currenciesDoc{Currencies: list.Items, Rates: rates.Items}, nil
		}

		sdk.AddTool(s, &sdk.Tool{Name: "list_currencies",
			Description: "Known currencies plus the latest exchange rates against the instance base currency."},
			func(ctx context.Context, req *sdk.CallToolRequest, in emptyInput) (*sdk.CallToolResult, currenciesDoc, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_currencies")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, currenciesDoc{}, err
				}
				doc, err := load(ctx, userID)
				if err != nil {
					return nil, currenciesDoc{}, webmcp.MapErr(ctx, err)
				}
				return nil, doc, nil
			})
	}
}
