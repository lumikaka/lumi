package realtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const (
	PhoenixHeartbeatTopic = "phoenix"
	SystemTopic           = "system"
	SystemStatusChanged   = "system:status_changed"
	OpenProjectChanged    = "open_project:changed"
)

type JoinError struct {
	Reason  string
	Message string
}

func (err *JoinError) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return err.Reason
}

type TopicAuthorizer interface {
	AuthorizeJoin(context.Context, string) (func(), error)
}

// PublicTopics allows only explicitly declared public infrastructure topics.
// Business topics must introduce their own authorization before being added.
type PublicTopics struct{}

func (PublicTopics) AuthorizeJoin(_ context.Context, topic string) (func(), error) {
	if strings.TrimSpace(topic) != SystemTopic {
		return nil, &JoinError{Reason: "invalid_topic", Message: "unsupported channel topic"}
	}
	return func() {}, nil
}

func ProjectTopic(projectUUID string) string { return "project:" + projectUUID }

type ProjectTopics struct {
	AcquirePresence func(string) (func(), error)
}

func (topics ProjectTopics) AuthorizeJoin(_ context.Context, topic string) (func(), error) {
	if topic == SystemTopic {
		return func() {}, nil
	}
	value, ok := strings.CutPrefix(strings.TrimSpace(topic), "project:")
	parsed, err := uuid.Parse(value)
	if !ok || err != nil || parsed.Version() != 7 {
		return nil, &JoinError{Reason: "invalid_topic", Message: "project topic must contain a UUIDv7"}
	}
	if topics.AcquirePresence == nil {
		return nil, &JoinError{Reason: "project_not_open", Message: "project topic is not open"}
	}
	release, err := topics.AcquirePresence(value)
	if err != nil {
		return nil, &JoinError{Reason: "project_not_open", Message: "project topic is not open"}
	}
	return release, nil
}
