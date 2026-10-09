// Package version exposes build-time version metadata for the /version endpoint
// and cross-service version coordination. Version/Commit/BuildTime are injected
// at build time via -ldflags (see Taskfile build:api); they keep sensible
// defaults for local `go run`.
package version

// Build metadata, overridden via -ldflags:
//
//	-X 'github.com/aspirit/xsocial/internal/version.Version=v1.2.3'
//	-X 'github.com/aspirit/xsocial/internal/version.Commit=abc1234'
//	-X 'github.com/aspirit/xsocial/internal/version.BuildTime=2026-05-30T10:00:00Z'
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// APIVersion is the API contract major version. Bump it on breaking API changes
// so clients can enforce a minimum supported API version.
const APIVersion = 1

// Info is the GET /version response payload.
type Info struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildTime  string `json:"buildTime"`
	APIVersion int    `json:"apiVersion"`
}

// Get returns the current build/version info.
func Get() Info {
	return Info{
		Version:    Version,
		Commit:     Commit,
		BuildTime:  BuildTime,
		APIVersion: APIVersion,
	}
}
