package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 45 * time.Second
	maxMessageSize = 64 * 1024
)

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	ctx    context.Context
	send   chan []byte
	done   chan struct{}
	once   sync.Once
	writer sync.WaitGroup
}

func newClient(ctx context.Context, hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub: hub, conn: conn, ctx: ctx,
		send: make(chan []byte, defaultSendQueueSize), done: make(chan struct{}),
	}
}

func (client *Client) run() {
	if !client.hub.register(client) {
		client.shutdown()
		_ = client.conn.Close()
		return
	}
	client.writer.Add(1)
	go client.writeLoop()
	client.readLoop()
	client.shutdown()
	client.writer.Wait()
}

func (client *Client) readLoop() {
	defer client.shutdown()
	client.conn.SetReadLimit(maxMessageSize)
	_ = client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		messageType, message, err := client.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			client.sendProtocolError("", nil, nil, "invalid_message_type", "channel frames must be text messages")
			continue
		}
		var frame Frame
		if err := json.Unmarshal(message, &frame); err != nil {
			client.sendProtocolError("", nil, nil, "invalid_json", "channel frame must be valid JSON")
			continue
		}
		client.handle(frame)
	}
}

func (client *Client) handle(frame Frame) {
	switch frame.Event {
	case EventHeartbeat:
		if frame.Topic != PhoenixHeartbeatTopic {
			client.sendProtocolError(frame.Topic, frame.Ref, frame.JoinRef, "invalid_topic", "heartbeat topic must be phoenix")
			return
		}
		client.trySend(replyFrame(frame, "ok", map[string]any{}))
	case EventJoin:
		client.handleJoin(frame)
	case EventLeave:
		client.handleLeave(frame)
	default:
		if _, joined := client.hub.joined(client, frame.Topic); !joined {
			client.sendProtocolError(frame.Topic, frame.Ref, frame.JoinRef, "not_joined", "topic has not been joined")
			return
		}
		client.sendProtocolError(frame.Topic, frame.Ref, frame.JoinRef, "unsupported_event", "client event is not supported")
	}
}

func (client *Client) handleJoin(frame Frame) {
	if frame.Ref == nil || *frame.Ref == "" {
		client.sendProtocolError(frame.Topic, frame.Ref, frame.JoinRef, "missing_ref", "join requires a ref")
		return
	}
	joinRef := *frame.Ref
	if frame.JoinRef != nil && *frame.JoinRef != "" {
		joinRef = *frame.JoinRef
	} else {
		frame.JoinRef = &joinRef
	}
	release, err := client.hub.authorizer.AuthorizeJoin(client.ctx, frame.Topic)
	if err != nil {
		response := ErrorResponse{Reason: "join_failed", Message: err.Error()}
		var joinError *JoinError
		if errors.As(err, &joinError) {
			response.Reason, response.Message = joinError.Reason, joinError.Message
		}
		client.trySend(replyFrame(frame, "error", response))
		return
	}
	if release == nil {
		release = func() {}
	}
	if !client.hub.join(client, frame.Topic, joinRef, release) {
		release()
		client.trySend(replyFrame(frame, "error", ErrorResponse{Reason: "join_failed", Message: "realtime hub is closed"}))
		return
	}
	client.trySend(replyFrame(frame, "ok", map[string]any{}))
}

func (client *Client) handleLeave(frame Frame) {
	joinRef, joined := client.hub.joined(client, frame.Topic)
	if frame.JoinRef == nil && joined {
		frame.JoinRef = &joinRef
	}
	client.hub.leave(client, frame.Topic)
	client.trySend(replyFrame(frame, "ok", map[string]any{}))
	client.trySend(Frame{
		Topic: frame.Topic, Event: EventClose, Payload: map[string]any{},
		Ref: frame.Ref, JoinRef: frame.JoinRef,
	})
}

func (client *Client) sendProtocolError(topic string, ref, joinRef *string, reason, message string) {
	client.trySend(Frame{
		Topic: topic, Event: EventError,
		Payload: ErrorResponse{Reason: reason, Message: message}, Ref: ref, JoinRef: joinRef,
	})
}

func (client *Client) trySend(frame Frame) bool {
	message, err := encodeFrame(frame)
	if err != nil {
		return false
	}
	select {
	case <-client.done:
		return false
	default:
	}
	select {
	case client.send <- message:
		return true
	default:
		return false
	}
}

func (client *Client) writeLoop() {
	defer client.writer.Done()
	defer func() {
		if client.conn != nil {
			_ = client.conn.Close()
		}
	}()
	defer recoverPanic("websocket writer", client.shutdown)
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case message := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				client.shutdown()
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				client.shutdown()
				return
			}
		case <-client.done:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			_ = client.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server closing"))
			return
		}
	}
}

func (client *Client) shutdown() {
	client.once.Do(func() {
		close(client.done)
		if client.hub != nil {
			client.hub.unregister(client)
		}
	})
}
