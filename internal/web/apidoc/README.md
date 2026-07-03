# API documentation (Swagger / OpenAPI)

This package holds the OpenAPI metadata for the Econumo Go API and serves it:

- **Swagger UI**: `GET /api/doc` (redirects to `/api/doc/`) — public, no JWT.
- **Raw spec**: `GET /api/doc.json` — public, no JWT.

(These paths match the PHP Nelmio bundle. The frontend does not consume them; the
spec is for humans/tooling and describes the endpoints accurately without being
byte-identical to Nelmio's output.)

## Files

- `doc.go` — general API info annotations (`@title`, `@version`,
  `securityDefinitions Bearer`) **and** the four response-envelope schemas
  (`JsonResponseOk` / `JsonResponseError` / `JsonResponseUnauthorized` /
  `JsonResponseException`) that the handler annotations reference.
- `serve.go` — `RegisterAPI()` returns a `router.RegisterAPI` that mounts the UI
  and the raw spec. It blank-imports `./docs` so the generated spec is registered
  with swaggo's global registry.
- `docs/` — **generated** by `swag init` (committed). Do not edit by hand.

## Regenerating the spec

The endpoint annotations live on the handlers in
`internal/ui/handler/category/*.go` and `internal/ui/handler/user/*.go`. swaggo
resolves the result types (which reference shared types across files in
`internal/app/category` and `internal/app/user`), so the generate command must
list all source dirs explicitly with `-d`:

```sh
# From internal/ui/apidoc:
go generate .

# …or equivalently, from internal/ui/apidoc:
go run github.com/swaggo/swag/cmd/swag@latest init \
  -g doc.go \
  -d .,../handler,../../app \
  -o ./docs \
  --parseInternal --parseDependency
```

The `-d` roots are scanned **recursively**: swag descends into every
`handler/<module>` and `app/<module>` subdirectory, so adding a new module
requires no change here — just annotate its handlers and re-run `go generate`.
`.` is this apidoc package (general info + the shared envelope schemas).
`--parseInternal --parseDependency` lets swaggo follow the cross-package type
references so nested field types (e.g. `CategoryResult`, `CurrentUserResult`)
resolve.

After regenerating, run `CGO_ENABLED=0 go build ./...` and `gofmt -l` to confirm
the tree is clean.
