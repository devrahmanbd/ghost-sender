package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"email-campaign-system/pkg/logger"
)

type Client struct {
	ID         string
	conn       *websocket.Conn
	hub        *Hub
	send       chan []byte
	log        logger.Logger
	RemoteAddr string
	mu         sync.RWMutex
	lastPong   time.Time
	closed     bool
}

type ClientMessage struct {
	Action string          `json:"action"`
	Topic  string          `json:"topic,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

func NewClient(id string, conn *websocket.Conn, hub *Hub, log logger.Logger, remoteAddr string) *Client { // FIX: was *logger.Logger
	return &Client{
		ID:         id,
		conn:       conn,
		hub:        hub,
		send:       make(chan []byte, 256),
		log:        log,
		RemoteAddr: remoteAddr,
		lastPong:   time.Now(),
		closed:     false,
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.close()
		c.hub.Unregister(c)
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.updateLastPong()
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	c.conn.SetReadLimit(maxMessageSize)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Warn("websocket read error",
					logger.String("client_id", c.ID),
					logger.Error(err),
				)
			}
			break
		}

		c.handleMessage(message)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write([]byte{'\n'})
				_, _ = w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(message []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Warn("invalid websocket message",
			logger.String("client_id", c.ID),
			logger.Error(err),
		)
		c.sendError("invalid message format")
		return
	}

	switch msg.Action {
	case "subscribe":
		c.handleSubscribe(msg.Topic)
	case "unsubscribe":
		c.handleUnsubscribe(msg.Topic)
	case "ping":
		c.handlePing()
	default:
		c.log.Warn("unknown action",
			logger.String("client_id", c.ID),
			logger.String("action", msg.Action),
		)
		c.sendError("unknown action: " + msg.Action)
	}
}

func (c *Client) handleSubscribe(topic string) {
	if topic == "" {
		c.sendError("topic required for subscribe")
		return
	}

	if !isValidTopic(topic) {
		c.sendError("invalid topic format")
		return
	}

	c.hub.Subscribe(c, topic)
	c.sendAck("subscribed", topic)
}

func (c *Client) handleUnsubscribe(topic string) {
	if topic == "" {
		c.sendError("topic required for unsubscribe")
		return
	}

	c.hub.Unsubscribe(c, topic)
	c.sendAck("unsubscribed", topic)
}

func (c *Client) handlePing() {
	c.sendAck("pong", "")
}

func (c *Client) sendAck(action string, topic string) {
	response := map[string]interface{}{
		"type":      "ack",
		"action":    action,
		"timestamp": time.Now(),
	}
	if topic != "" {
		response["topic"] = topic
	}

	data, err := json.Marshal(response)
	if err != nil {
		c.log.Error("failed to marshal ack",
			logger.String("client_id", c.ID),
			logger.Error(err),
		)
		return
	}

	select {
	case c.send <- data:
	default:
		c.log.Warn("send buffer full, dropping ack",
			logger.String("client_id", c.ID),
		)
	}
}

func (c *Client) sendError(errMsg string) {
	response := map[string]interface{}{
		"type":      "error",
		"error":     errMsg,
		"timestamp": time.Now(),
	}

	data, err := json.Marshal(response)
	if err != nil {
		c.log.Error("failed to marshal error",
			logger.String("client_id", c.ID),
			logger.Error(err),
		)
		return
	}

	select {
	case c.send <- data:
	default:
		c.log.Warn("send buffer full, dropping error",
			logger.String("client_id", c.ID),
		)
	}
}

func (c *Client) Send(message []byte) {
	if c.IsClosed() {
		return
	}

	select {
	case c.send <- message:
	default:
		c.log.Warn("send buffer full, dropping message",
			logger.String("client_id", c.ID),
		)
	}
}

func (c *Client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	_ = c.conn.Close()
}

func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

func (c *Client) IsAlive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return false
	}

	return time.Since(c.lastPong) < pongWait*2
}

func (c *Client) updateLastPong() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPong = time.Now()
}

func (c *Client) GetLastPong() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastPong
}
