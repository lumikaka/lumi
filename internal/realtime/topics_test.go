package realtime

import (
	"context"
	"errors"
	"testing"
)

func TestProjectTopicsAuthorizeOpenUUIDv7ProjectsAndLeasePresence(t *testing.T) {
	t.Parallel()
	first := "01989abc-def0-7000-8000-000000000001"
	second := "01989abc-def0-7000-8000-000000000002"
	open := map[string]bool{first: true, second: true}
	leasing := 0
	authorizer := ProjectTopics{AcquirePresence: func(projectUUID string) (func(), error) {
		if !open[projectUUID] {
			return nil, errors.New("not open")
		}
		leasing++
		return func() { leasing-- }, nil
	}}
	for _, topic := range []string{SystemTopic, ProjectTopic(first), ProjectTopic(second)} {
		release, err := authorizer.AuthorizeJoin(context.Background(), topic)
		if err != nil {
			t.Fatalf("topic %q rejected: %v", topic, err)
		}
		release()
	}
	if leasing != 0 {
		t.Fatalf("presence leases = %d", leasing)
	}
	tests := []struct {
		topic  string
		reason string
	}{
		{topic: "project:1", reason: "invalid_topic"},
		{topic: "project:01989abc-def0-4000-8000-000000000001", reason: "invalid_topic"},
		{topic: "project:01989abc-def0-7000-8000-000000000003", reason: "project_not_open"},
		{topic: "other:" + first, reason: "invalid_topic"},
	}
	for _, test := range tests {
		_, err := authorizer.AuthorizeJoin(context.Background(), test.topic)
		var joinErr *JoinError
		if !errors.As(err, &joinErr) || joinErr.Reason != test.reason {
			t.Fatalf("topic %q error = %#v", test.topic, err)
		}
	}
}
