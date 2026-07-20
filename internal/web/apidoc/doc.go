// Package apidoc holds the OpenAPI/Swagger metadata for the Econumo API: the
// general API info annotations (consumed by swaggo/swag to generate docs.go) and
// the four response-envelope schemas the handler annotations reference. The
// generated docs.go is committed; regenerate it with `make swagger` (the
// canonical, version-pinned path — also run automatically before
// build/run/publish/docker-up and verified by `make go-lint`/`make go-test`). For an
// ad-hoc run use `go generate ./internal/web/apidoc`.
//
// Serving: ui/router wires Swagger UI at /api/doc and the raw spec at
// /api/doc.json (both public; paths are wire-compatible with existing clients,
// see CLAUDE.md). The frontend does not consume these; the spec is for
// human/tooling use, so it describes the endpoints accurately.
//
// @title                      Econumo API
// @version                    1.0.0
// @description                Self-hosted personal finance and budgeting API
// @BasePath                   /
// @securityDefinitions.apikey Bearer
// @in                         header
// @name                       Authorization
// @description                Opaque access token. Format: "Bearer <token>".
//
// Recursive scan of the package roots: swag descends into every subdirectory of
// ../handler (handler/<module>) and ../../app (app/<module>), so a NEW module's
// handlers + DTOs are picked up automatically — no per-module -d entry to add.
// `.` is this apidoc package (general info + the shared envelope schemas).
//
//go:generate go run github.com/swaggo/swag/cmd/swag@latest init -g doc.go -d .,../handler,../../app -o ./docs --parseInternal --parseDependency
package apidoc

// JsonResponseOk is the success envelope. data is endpoint-specific; handler
// annotations refine it via the {data=...} composition.
type JsonResponseOk struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:""`
	Data    interface{} `json:"data"`
}

// JsonResponseError is the validation/handled-error envelope (HTTP 400). errors
// is a map of field name -> list of messages.
type JsonResponseError struct {
	Success bool                `json:"success" example:"false"`
	Message string              `json:"message" example:"Validation failed"`
	Code    int                 `json:"code" example:"0"`
	Errors  map[string][]string `json:"errors"`
}

// JsonResponseUnauthorized is the 401 envelope (missing/invalid access token).
type JsonResponseUnauthorized struct {
	Success bool                `json:"success" example:"false"`
	Message string              `json:"message" example:"Access token not found"`
	Code    int                 `json:"code" example:"0"`
	Errors  map[string][]string `json:"errors"`
}

// JsonResponseException is the 500 unhandled-exception envelope. stackTrace is
// present only in dev.
type JsonResponseException struct {
	Success       bool        `json:"success" example:"false"`
	Message       string      `json:"message" example:"Internal Server Error"`
	Code          int         `json:"code" example:"0"`
	ExceptionType string      `json:"exceptionType,omitempty"`
	StackTrace    interface{} `json:"stackTrace,omitempty"`
}
