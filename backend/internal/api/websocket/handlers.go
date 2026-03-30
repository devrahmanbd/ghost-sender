package websocket

import (
        crypto_rand "crypto/rand"
        "encoding/hex"
        "encoding/json"
        "net/http"
        "strings"
        "time"

        "github.com/gorilla/websocket"

        "email-campaign-system/internal/api/middleware"
        "email-campaign-system/internal/config"
        "email-campaign-system/pkg/logger"
)

const (
        maxMessageSize = 8192
        writeWait      = 10 * time.Second
        pongWait       = 60 * time.Second
        pingPeriod     = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 4096,
        CheckOrigin:     checkOrigin,
}

type Handler struct {
        hub *Hub
        log logger.Logger // FIX: was *logger.Logger
}

func NewHandler(hub *Hub, log logger.Logger) *Handler { // FIX: was *logger.Logger
        return &Handler{
                hub: hub,
                log: log,
        }
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                h.log.Error("websocket upgrade failed", logger.Error(err))
                return
        }

        topics := h.parseTopics(r)
        clientID := h.getClientID(r)
        remoteAddr := r.RemoteAddr

        client := NewClient( // NewClient now expects logger.Logger
                clientID,
                conn,
                h.hub,
                h.log,
                remoteAddr,
        )

        h.hub.Register(client)

        for _, topic := range topics {
                h.hub.Subscribe(client, topic)
        }

        go client.WritePump()
        go client.ReadPump()

        h.log.Info("websocket connection established",
                logger.String("client_id", clientID),
                logger.String("remote_addr", remoteAddr),
                logger.Any("topics", topics),
        )
}

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
        stats := h.hub.GetStats()

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(map[string]interface{}{
                "status": "ok",
                "data":   stats,
        })
}

func (h *Handler) parseTopics(r *http.Request) []string {
        q := r.URL.Query()

        topicsParam := q.Get("topics")
        if topicsParam == "" {
                return []string{}
        }

        parts := strings.Split(topicsParam, ",")
        topics := make([]string, 0, len(parts))

        for _, p := range parts {
                topic := strings.TrimSpace(p)
                if topic != "" && isValidTopic(topic) {
                        topics = append(topics, topic)
                }
        }

        return topics
}

func (h *Handler) getClientID(r *http.Request) string {
        if rid := middleware.GetRequestID(r.Context()); rid != "" {
                return rid
        }

        q := r.URL.Query()
        if cid := q.Get("client_id"); cid != "" {
                return sanitizeClientID(cid)
        }

        return generateClientID()
}

func isValidTopic(topic string) bool {
        if len(topic) < 1 || len(topic) > 128 {
                return false
        }

        for i := 0; i < len(topic); i++ {
                c := topic[i]
                if (c >= 'a' && c <= 'z') ||
                        (c >= 'A' && c <= 'Z') ||
                        (c >= '0' && c <= '9') ||
                        c == '-' || c == '_' || c == ':' || c == '.' || c == '*' {
                        continue
                }
                return false
        }

        return true
}

func sanitizeClientID(id string) string {
        if len(id) > 64 {
                id = id[:64]
        }

        sanitized := make([]byte, 0, len(id))
        for i := 0; i < len(id); i++ {
                c := id[i]
                if (c >= 'a' && c <= 'z') ||
                        (c >= 'A' && c <= 'Z') ||
                        (c >= '0' && c <= '9') ||
                        c == '-' || c == '_' {
                        sanitized = append(sanitized, c)
                }
        }

        if len(sanitized) == 0 {
                return generateClientID()
        }

        return string(sanitized)
}

func generateClientID() string {
        return "ws-" + randomHex(16)
}

func randomHex(n int) string {
        b := make([]byte, n)
        if _, err := crypto_rand.Read(b); err != nil {
                for i := range b {
                        b[i] = byte(time.Now().UnixNano() & 0xFF)
                }
        }
        return hex.EncodeToString(b)[:n]
}

func checkOrigin(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        if origin == "" {
                return true
        }

        cfg := config.Get()
        if cfg == nil {
                return false
        }

        allowedOrigins := cfg.Server.AllowedOrigins
        if len(allowedOrigins) == 0 {
                return true
        }

        for _, allowed := range allowedOrigins {
                if allowed == "*" {
                        return true
                }
                if strings.EqualFold(origin, allowed) {
                        return true
                }
        }

        return false
}
