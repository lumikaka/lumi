package realtime

import "encoding/json"

const (
	EventJoin      = "phx_join"
	EventLeave     = "phx_leave"
	EventReply     = "phx_reply"
	EventError     = "phx_error"
	EventClose     = "phx_close"
	EventHeartbeat = "heartbeat"
)

// Frame is the application realtime wire protocol. It is intentionally
// separate from the REST API JSON envelope.
type Frame struct {
	Topic   string  `json:"topic"`
	Event   string  `json:"event"`
	Payload any     `json:"payload"`
	Ref     *string `json:"ref"`
	JoinRef *string `json:"join_ref"`
}

type ReplyPayload struct {
	Status   string `json:"status"`
	Response any    `json:"response"`
}

type ErrorResponse struct {
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

func replyFrame(in Frame, status string, response any) Frame {
	return Frame{
		Topic: in.Topic, Event: EventReply,
		Payload: ReplyPayload{Status: status, Response: response},
		Ref:     in.Ref, JoinRef: in.JoinRef,
	}
}

func encodeFrame(frame Frame) ([]byte, error) {
	return json.Marshal(frame)
}
