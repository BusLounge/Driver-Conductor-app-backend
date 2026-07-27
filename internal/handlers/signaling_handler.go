package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/smarttransit/sms-auth-backend/internal/models"
	"github.com/smarttransit/sms-auth-backend/internal/services"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin requests for WebSockets
	},
}

// SignalingHandler handles WebRTC signaling WebSocket connections
type SignalingHandler struct {
	signalingService *services.SignalingService
}

// NewSignalingHandler creates a new instance of SignalingHandler
func NewSignalingHandler(svc *services.SignalingService) *SignalingHandler {
	return &SignalingHandler{
		signalingService: svc,
	}
}

// HandleWebSocket upgrades HTTP connection to WebSocket and manages communication
func (h *SignalingHandler) HandleWebSocket(c *gin.Context) {
	roomID := c.Query("room_id")
	senderID := c.Query("client_id")
	role := c.Query("role")

	if roomID == "" || senderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room_id and client_id query parameters are required"})
		return
	}

	if role == "" {
		role = "peer"
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.Errorf("WebRTC Signaling: Failed to upgrade connection: %v", err)
		return
	}

	client := &services.SignalingClient{
		ID:       senderID,
		RoomID:   roomID,
		Role:     role,
		Conn:     conn,
		SendChan: make(chan []byte, 256),
	}

	h.signalingService.RegisterClient(client)

	// Launch write pump goroutine
	go h.writePump(client)

	// Read pump on main goroutine
	h.readPump(client)
}

func (h *SignalingHandler) readPump(client *services.SignalingClient) {
	defer func() {
		h.signalingService.UnregisterClient(client)
		client.Conn.Close()
	}()

	client.Conn.SetReadLimit(512 * 1024)
	client.Conn.SetReadDeadline(time.Now().Add(pongWait))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logrus.Warnf("WebRTC Signaling: Client %s read error: %v", client.ID, err)
			}
			break
		}

		var msg models.SignalingMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			logrus.Errorf("WebRTC Signaling: Invalid JSON format from client %s: %v", client.ID, err)
			continue
		}

		// Enforce client ID & room ID consistency
		msg.SenderID = client.ID
		msg.RoomID = client.RoomID
		msg.Role = client.Role

		h.signalingService.RouteMessage(msg)
	}
}

func (h *SignalingHandler) writePump(client *services.SignalingClient) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.SendChan:
			client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
