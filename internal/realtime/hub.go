package realtime

import "sync"

const defaultSendQueueSize = 64

type membership struct {
	client  *Client
	joinRef string
}

type topicMembership struct {
	joinRef string
	release func()
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	topics     map[string]map[*Client]topicMembership
	authorizer TopicAuthorizer
	closed     bool
}

func NewHub(authorizer TopicAuthorizer) *Hub {
	if authorizer == nil {
		authorizer = PublicTopics{}
	}
	return &Hub{
		clients: make(map[*Client]struct{}), topics: make(map[string]map[*Client]topicMembership),
		authorizer: authorizer,
	}
}

func (hub *Hub) register(client *Client) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.closed {
		return false
	}
	hub.clients[client] = struct{}{}
	return true
}

func (hub *Hub) unregister(client *Client) {
	hub.mu.Lock()
	delete(hub.clients, client)
	var releases []func()
	for topic, members := range hub.topics {
		if member, ok := members[client]; ok && member.release != nil {
			releases = append(releases, member.release)
		}
		delete(members, client)
		if len(members) == 0 {
			delete(hub.topics, topic)
		}
	}
	hub.mu.Unlock()
	for _, release := range releases {
		release()
	}
}

func (hub *Hub) join(client *Client, topic, joinRef string, release func()) bool {
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return false
	}
	members := hub.topics[topic]
	if members == nil {
		members = make(map[*Client]topicMembership)
		hub.topics[topic] = members
	}
	previous := members[client]
	members[client] = topicMembership{joinRef: joinRef, release: release}
	hub.mu.Unlock()
	if previous.release != nil {
		previous.release()
	}
	return true
}

func (hub *Hub) leave(client *Client, topic string) {
	hub.mu.Lock()
	members := hub.topics[topic]
	member := members[client]
	delete(members, client)
	if len(members) == 0 {
		delete(hub.topics, topic)
	}
	hub.mu.Unlock()
	if member.release != nil {
		member.release()
	}
}

func (hub *Hub) joined(client *Client, topic string) (string, bool) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	member, ok := hub.topics[topic][client]
	return member.joinRef, ok
}

func (hub *Hub) Broadcast(topic, event string, payload any) {
	// A nil *Hub can cross an EventPublisher interface boundary as a non-nil
	// interface value. Treat disabled realtime delivery as a no-op.
	if hub == nil {
		return
	}
	hub.mu.RLock()
	members := make([]membership, 0, len(hub.topics[topic]))
	for client, member := range hub.topics[topic] {
		members = append(members, membership{client: client, joinRef: member.joinRef})
	}
	hub.mu.RUnlock()

	for _, member := range members {
		joinRef := member.joinRef
		if !member.client.trySend(Frame{
			Topic: topic, Event: event, Payload: payload, Ref: nil, JoinRef: &joinRef,
		}) {
			client := member.client
			go runSafely("slow websocket client shutdown", func() {
				if client != nil {
					client.shutdown()
				}
			})
		}
	}
}

func (hub *Hub) MembershipCount(topic string) int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.topics[topic])
}

func (hub *Hub) ConnectionCount() int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}

func (hub *Hub) Close() error {
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return nil
	}
	hub.closed = true
	clients := make([]*Client, 0, len(hub.clients))
	for client := range hub.clients {
		clients = append(clients, client)
	}
	hub.mu.Unlock()

	for _, client := range clients {
		client.shutdown()
	}
	return nil
}
