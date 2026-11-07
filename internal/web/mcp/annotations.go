package mcp

import sdk "github.com/modelcontextprotocol/go-sdk/mcp"

// Tool annotations. Claude's connector-directory review requires a title and
// the applicable hint on every tool, and the hints drive auto-permissions in
// the client: a read-only tool runs without a per-call confirmation, a
// destructive one always prompts. The MCP default for DestructiveHint is TRUE
// whenever ReadOnlyHint is false, so an additive or reversible write MUST
// declare it false explicitly or every one of its calls prompts the user.
//
// Title is set in both places the spec allows (Tool.Title and
// Annotations.Title, whose display precedence is title -> annotations.title ->
// name) so a client or review scanner reading either one finds it.
//
// OpenWorldHint is false throughout: every Econumo tool reads or writes the
// caller's own ledger, a closed domain, and reaches no external service.
func annotate(name, title, description string, readOnly, destructive bool) *sdk.Tool {
	a := &sdk.ToolAnnotations{Title: title, ReadOnlyHint: readOnly, OpenWorldHint: ptr(false)}
	if !readOnly {
		// Meaningful only when ReadOnlyHint is false, and it defaults to true.
		a.DestructiveHint = ptr(destructive)
	}
	return &sdk.Tool{Name: name, Title: title, Description: description, Annotations: a}
}

func ptr(b bool) *bool { return &b }

// ReadOnlyTool returns a tool that only reads the caller's data, so the client
// may run it without asking.
func ReadOnlyTool(name, title, description string) *sdk.Tool {
	return annotate(name, title, description, true, false)
}

// WriteTool returns a tool whose writes are additive or reversible in the app
// (a create, a rename, an archive that unarchives, a single-row update), so
// the client need not confirm every call.
func WriteTool(name, title, description string) *sdk.Tool {
	return annotate(name, title, description, false, false)
}

// DestructiveTool returns a tool that deletes data or overwrites it in bulk,
// so the client prompts before every call.
func DestructiveTool(name, title, description string) *sdk.Tool {
	return annotate(name, title, description, false, true)
}
