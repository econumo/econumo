// Package mcp is the MCP edge shared by every feature: it builds the SDK
// server, mounts the Streamable HTTP transport (stateless + JSON responses:
// tools are sub-second DB calls with nothing to stream, and statelessness
// keeps /mcp restart-safe and proxy-friendly), and defines the seam through
// which feature packages register their tools/resources/prompts.
package mcp

import (
	"net/http"
	"runtime/debug"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Register func(*sdk.Server)

func Compose(fns ...Register) Register {
	return func(s *sdk.Server) {
		for _, fn := range fns {
			if fn != nil {
				fn(s)
			}
		}
	}
}

const instructions = "Econumo personal-finance server. Read reference data from the econumo:// " +
	"resources (accounts, categories, tags, payees, currencies, budgets, user); query monthly " +
	"budget state with get_budget and transactions with list_transactions; log changes with " +
	"create_transaction / update_transaction / delete_transaction. Amounts are decimal strings; " +
	"ids are UUIDs from the resources."

func NewHandler(register Register) http.Handler {
	srv := sdk.NewServer(
		&sdk.Implementation{Name: "econumo", Version: serverVersion()},
		&sdk.ServerOptions{Instructions: instructions},
	)
	register(srv)
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return srv },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}

func serverVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
