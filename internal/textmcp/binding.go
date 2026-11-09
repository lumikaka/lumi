package textmcp

import (
	"context"
	"regexp"
)

const ExpectedProjectUUIDMeta = "lumi/expected_project_uuid"

var publicProjectUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-7[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type expectedProjectKey struct{}

// WithExpectedProjectUUID adds a constraint, not an authorization binding.
// The trusted caller still supplies the directory independently of tool input.
func WithExpectedProjectUUID(ctx context.Context, uuid string) context.Context {
	return context.WithValue(ctx, expectedProjectKey{}, uuid)
}

func ExpectedProjectUUID(ctx context.Context) string {
	uuid, _ := ctx.Value(expectedProjectKey{}).(string)
	return uuid
}

// Host identifies the MCP host that launched `lumi mcp --host`. It is a
// transport hint used only for tool gating and diagnostics: it is never
// persisted, never authorizes access and never appears in realtime payloads.
const (
	HostClaude = "claude"
	HostCodex  = "codex"
	HostCindy  = "cindy"
)

// ValidHost accepts the supported --host values; "" means an unknown host
// and keeps the historical behavior.
func ValidHost(host string) bool {
	return host == "" || host == HostClaude || host == HostCodex || host == HostCindy
}

type hostKey struct{}

func WithHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, hostKey{}, host)
}

func HostFromContext(ctx context.Context) string {
	host, _ := ctx.Value(hostKey{}).(string)
	return host
}
