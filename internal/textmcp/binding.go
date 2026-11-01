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
