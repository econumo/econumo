// Package mcp is the MCP edge shared by every feature: it builds the SDK
// server, mounts the Streamable HTTP transport (stateless + JSON responses:
// tools are sub-second DB calls with nothing to stream, and statelessness
// keeps /mcp restart-safe and proxy-friendly), and defines the seam through
// which feature packages register their tools/prompts.
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

const instructions = "Econumo personal-finance server. Read reference data with the list_accounts, " +
	"list_categories, list_tags, list_payees, list_currencies, list_budgets and get_user tools; " +
	"query monthly budget state with get_budget and transactions with list_transactions; log " +
	"changes with create_transaction / update_transaction / delete_transaction. Manage accounts " +
	"and full budget structure (folders, envelopes, limits, moves) with create_account and the " +
	"budget write tools. Amounts are decimal strings; ids are UUIDs from the list_* tools."

func NewHandler(register Register) http.Handler {
	srv := sdk.NewServer(
		&sdk.Implementation{Name: "econumo", Version: serverVersion()},
		&sdk.ServerOptions{Instructions: instructions},
	)
	register(srv)
	addPrompts(srv)
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return srv },
		// DisableLocalhostProtection: /mcp sits behind a reverse proxy that
		// dials the app over loopback, so the SDK's DNS-rebinding guard (added
		// in v1.4.0) sees a loopback local address with a public Host header
		// and 403s every request. That guard defends browser-driven local
		// desktop servers with ambient credentials; this edge is a public
		// HTTPS endpoint authenticated by a bearer PAT, so the threat model
		// does not apply.
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true},
	)
}

func serverVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
