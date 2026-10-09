# Internal errors package

This package holds **generic / infrastructure** error sentinels used across the application. Domain-specific errors live in `internal/domains/<domain>/errors.go`.

## Layout

- **storage.go** – storage / persistence (not found, conflict, MongoDB and Redis client configuration).
- **validation.go** – request validation (`ErrInvalidParams`, `ErrRequestLimitExceeded`), rate-limit and JWT-key configuration checks, and the JSON-binding error helper.
- **system.go** – event broker, service configuration, and the generic `ErrNotImplemented` / `ErrInternalError`.  
  Tools (`tools/mongo-codegen`) and `pkg/*` define their own sentinels; never duplicate them here.

## Where to define new errors

- **Domain semantics** (auth, post, notification, ...) → `internal/domains/<domain>/errors.go`.
- **Reusable / project-root semantics** (e.g. go.mod not found) → `pkg/project` or the relevant `pkg/` package.
- **Generic validation / storage / system** → this package under the appropriate file.

Do **not** introduce new sentinels that duplicate the semantics of an existing error in this package or in a domain/pkg.

## Converter

`internal/apierrors/converter.go` (generic) and `domain_converter.go` (domain sentinels) map internal and domain errors to API errors. When adding or moving errors, update the converter so that `errors.Is(err, yourErr)` is handled and mapped to the desired API response.
