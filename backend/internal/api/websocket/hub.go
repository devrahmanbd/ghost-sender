package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"email-campaign-system/pkg/logger"
)

type MessageType string

const (
	MessageTypeCampaignProgress MessageType = "campaign_progress"
	MessageTypeCampaignStatus   MessageType = "campaign_status"
	MessageTypeSystemMetrics    MessageType = "system_metrics"
	MessageTypeAccountStatus    MessageType = "account_status"
	MessageTypeLogs             MessageType = "logs"
	MessageTypeNotification     MessageType = "notification"
	MessageTypeError            MessageType = "error"
)

type Message struct {
	Type       MessageType     `json:"type"`
	Topic      string          `json:"topic,omitempty"`
	CampaignID string          `json:"campaign_id,omitempty"`
	Data       json.RawMessage `json:"data"`
	Timestamp  time.Time       `json:"timestamp"`
}

type Hub struct {
	clients         map[*Client]bool
	clientsMu       sync.RWMutex
	subscriptions   map[string]map[*Client]bool
	subscriptionsMu sync.RWMutex
	broadcast       chan *Message
	register        chan *Client
	unregister      chan *Client
	subscribe       chan *subscription
	unsubscribe     chan *subscription
	stop            chan struct{}
	wg              sync.WaitGroup
	log             logger.Logger // FIX: was *logger.Logger
}

type subscription struct {
	client *Client
	topic  string
}

func NewHub(log logger.Logger) *Hub { // FIX: was *logger.Logger
	return &Hub{
		clients:       make(map[*Client]bool),
		subscriptions: make(map[string]map[*Client]bool),
		broadcast:     make(chan *Message, 256),
		register:      make(chan *Client, 16),
		unregister:    make(chan *Client, 16),
		subscribe:     make(chan *subscription, 16),
		unsubscribe:   make(chan *subscription, 16),
		stop:          make(chan struct{}),
		log:           log,
	}
}

func (h *Hub) Start(ctx context.Context) {
	h.wg.Add(1)
	go h.run(ctx)
}

func (h *Hub) Stop() {
	close(h.stop)
	h.wg.Wait()
}

func (h *Hub) run(ctx context.Context) {
	defer h.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return

		case <-h.stop:
			h.shutdown()
			return

		case client := <-h.register:
			h.handleRegister(client)

		case client := <-h.unregister:
			h.handleUnregister(client)

		case sub := <-h.subscribe:
			h.handleSubscribe(sub)

		case unsub := <-h.unsubscribe:
			h.handleUnsubscribe(unsub)

		case msg := <-h.broadcast:
			h.handleBroadcast(msg)

		case <-ticker.C:
			h.cleanupDeadClients()
		}
	}
}

func (h *Hub) handleRegister(client *Client) {
	h.clientsMu.Lock()
	h.clients[client] = true
	h.clientsMu.Unlock()

	h.log.Info("websocket client registered",
		logger.String("client_id", client.ID),
		logger.String("remote_addr", client.RemoteAddr),
		logger.Int("total_clients", len(h.clients)),
	)
}

func (h *Hub) handleUnregister(client *Client) {
	h.clientsMu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.clientsMu.Unlock()

	h.subscriptionsMu.Lock()
	for topic := range h.subscriptions {
		delete(h.subscriptions[topic], client)
		if len(h.subscriptions[topic]) == 0 {
			delete(h.subscriptions, topic)
		}
	}
	h.subscriptionsMu.Unlock()

	h.log.Info("websocket client unregistered",
		logger.String("client_id", client.ID),
		logger.String("remote_addr", client.RemoteAddr),
		logger.Int("total_clients", len(h.clients)),
	)
}

func (h *Hub) handleSubscribe(sub *subscription) {
	h.subscriptionsMu.Lock()
	if h.subscriptions[sub.topic] == nil {
		h.subscriptions[sub.topic] = make(map[*Client]bool)
	}
	h.subscriptions[sub.topic][sub.client] = true
	h.subscriptionsMu.Unlock()

	h.log.Debug("client subscribed to topic",
		logger.String("client_id", sub.client.ID),
		logger.String("topic", sub.topic),
	)
}

func (h *Hub) handleUnsubscribe(unsub *subscription) {
	h.subscriptionsMu.Lock()
	if clients, ok := h.subscriptions[unsub.topic]; ok {
		delete(clients, unsub.client)
		if len(clients) == 0 {
			delete(h.subscriptions, unsub.topic)
		}
	}
	h.subscriptionsMu.Unlock()

	h.log.Debug("client unsubscribed from topic",
		logger.String("client_id", unsub.client.ID),
		logger.String("topic", unsub.topic),
	)
}

func (h *Hub) handleBroadcast(msg *Message) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("failed to marshal broadcast message", logger.Error(err))
		return
	}

	if msg.Topic != "" {
		h.broadcastToTopic(msg.Topic, data)
		return
	}

	h.broadcastToAll(data)
}

func (h *Hub) broadcastToAll(data []byte) {
	h.clientsMu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.clientsMu.RUnlock()

	for _, client := range clients {
		select {
		case client.send <- data:
		default:
			h.log.Warn("client send buffer full, dropping message",
				logger.String("client_id", client.ID),
			)
		}
	}
}

func (h *Hub) broadcastToTopic(topic string, data []byte) {
	h.subscriptionsMu.RLock()
	clients, ok := h.subscriptions[topic]
	if !ok {
		h.subscriptionsMu.RUnlock()
		return
	}

	clientList := make([]*Client, 0, len(clients))
	for client := range clients {
		clientList = append(clientList, client)
	}
	h.subscriptionsMu.RUnlock()

	for _, client := range clientList {
		select {
		case client.send <- data:
		default:
			h.log.Warn("client send buffer full, dropping message",
				logger.String("client_id", client.ID),
				logger.String("topic", topic),
			)
		}
	}
}

func (h *Hub) cleanupDeadClients() {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	for client := range h.clients {
		if !client.IsAlive() {
			delete(h.clients, client)
			close(client.send)
		}
	}
}

func (h *Hub) shutdown() {
	h.log.Info("shutting down websocket hub")

	h.clientsMu.Lock()
	for client := range h.clients {
		close(client.send)
	}
	h.clients = make(map[*Client]bool)
	h.clientsMu.Unlock()

	h.subscriptionsMu.Lock()
	h.subscriptions = make(map[string]map[*Client]bool)
	h.subscriptionsMu.Unlock()
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) Subscribe(client *Client, topic string) {
	h.subscribe <- &subscription{client: client, topic: topic}
}

func (h *Hub) Unsubscribe(client *Client, topic string) {
	h.unsubscribe <- &subscription{client: client, topic: topic}
}

func (h *Hub) Broadcast(msg *Message) {
	select {
	case h.broadcast <- msg:
	default:
		h.log.Warn("broadcast channel full, dropping message",
			logger.String("type", string(msg.Type)),
			logger.String("topic", msg.Topic),
		)
	}
}

func (h *Hub) BroadcastCampaignProgress(campaignID string, data interface{}) {
	h.broadcastTyped(MessageTypeCampaignProgress, "campaign:"+campaignID, campaignID, data)
}

func (h *Hub) BroadcastCampaignStatus(campaignID string, data interface{}) {
	h.broadcastTyped(MessageTypeCampaignStatus, "campaign:"+campaignID, campaignID, data)
}

func (h *Hub) BroadcastSystemMetrics(data interface{}) {
	h.broadcastTyped(MessageTypeSystemMetrics, "system", "", data)
}

func (h *Hub) BroadcastAccountStatus(accountID string, data interface{}) {
	h.broadcastTyped(MessageTypeAccountStatus, "account:"+accountID, "", data)
}

func (h *Hub) BroadcastLogs(campaignID string, data interface{}) {
	topic := "logs"
	if campaignID != "" {
		topic = "logs:" + campaignID
	}
	h.broadcastTyped(MessageTypeLogs, topic, campaignID, data)
}

func (h *Hub) BroadcastNotification(data interface{}) {
	h.broadcastTyped(MessageTypeNotification, "", "", data)
}

func (h *Hub) BroadcastError(err error, campaignID string) {
	data := map[string]interface{}{
		"error":   err.Error(),
		"message": "An error occurred",
	}
	h.broadcastTyped(MessageTypeError, "", campaignID, data)
}

func (h *Hub) broadcastTyped(msgType MessageType, topic string, campaignID string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		h.log.Error("failed to marshal broadcast data",
			logger.String("type", string(msgType)),
			logger.Error(err),
		)
		return
	}

	msg := &Message{
		Type:       msgType,
		Topic:      topic,
		CampaignID: campaignID,
		Data:       jsonData,
		Timestamp:  time.Now(),
	}

	h.Broadcast(msg)
}

func (h *Hub) GetStats() map[string]interface{} {
	h.clientsMu.RLock()
	totalClients := len(h.clients)
	h.clientsMu.RUnlock()

	h.subscriptionsMu.RLock()
	topics := make([]string, 0, len(h.subscriptions))
	topicCounts := make(map[string]int)
	for topic, clients := range h.subscriptions {
		topics = append(topics, topic)
		topicCounts[topic] = len(clients)
	}
	h.subscriptionsMu.RUnlock()

	return map[string]interface{}{
		"total_clients":   totalClients,
		"total_topics":    len(topics),
		"topic_counts":    topicCounts,
		"broadcast_queue": len(h.broadcast),
	}
}
