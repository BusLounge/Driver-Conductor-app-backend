package services

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// SignalingClient represents a single connected peer
type SignalingClient struct {
	ID       string
	RoomID   string
	Role     string
	Conn     *websocket.Conn
	SendChan chan []byte
}

// SignalingService manages signaling rooms and client routing
type SignalingService struct {
	// rooms maps roomID -> map of clientID -> *SignalingClient
	rooms map[string]map[string]*SignalingClient
	mu    sync.RWMutex
}

// NewSignalingService creates a new instance of SignalingService
func NewSignalingService() *SignalingService {
	return &SignalingService{
		rooms: make(map[string]map[string]*SignalingClient),
	}
}

// RegisterClient adds a client to a signaling room
func (s *SignalingService) RegisterClient(client *SignalingClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[client.RoomID]; !exists {
		s.rooms[client.RoomID] = make(map[string]*SignalingClient)
	}

	s.rooms[client.RoomID][client.ID] = client
	logrus.Infof("WebRTC Signaling: Client %s (%s) joined room %s", client.ID, client.Role, client.RoomID)

	// Broadcast join message to existing peers in room
	joinMsg := models.SignalingMessage{
		Type:     models.MessageTypeJoin,
		RoomID:   client.RoomID,
		SenderID: client.ID,
		Role:     client.Role,
	}
	s.broadcastToRoom(client.RoomID, client.ID, joinMsg)
}

// UnregisterClient removes a client from a room and closes connection
func (s *SignalingService) UnregisterClient(client *SignalingClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if room, exists := s.rooms[client.RoomID]; exists {
		if _, ok := room[client.ID]; ok {
			delete(room, client.ID)
			close(client.SendChan)
			logrus.Infof("WebRTC Signaling: Client %s left room %s", client.ID, client.RoomID)

			leaveMsg := models.SignalingMessage{
				Type:     models.MessageTypeLeave,
				RoomID:   client.RoomID,
				SenderID: client.ID,
				Role:     client.Role,
			}
			s.broadcastToRoom(client.RoomID, client.ID, leaveMsg)
		}
		if len(room) == 0 {
			delete(s.rooms, client.RoomID)
		}
	}
}

// RouteMessage forwards a signaling message to target or room
func (s *SignalingService) RouteMessage(msg models.SignalingMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[msg.RoomID]
	if !exists {
		return
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		logrus.Errorf("WebRTC Signaling: Error marshaling message: %v", err)
		return
	}

	// Direct message to target client if target_id specified
	if msg.TargetID != "" {
		if targetClient, ok := room[msg.TargetID]; ok {
			select {
			case targetClient.SendChan <- raw:
			default:
				logrus.Warnf("WebRTC Signaling: Client %s send buffer full", targetClient.ID)
			}
		}
		return
	}

	// Broadcast to all other peers in room
	for id, client := range room {
		if id != msg.SenderID {
			select {
			case client.SendChan <- raw:
			default:
				logrus.Warnf("WebRTC Signaling: Client %s send buffer full", id)
			}
		}
	}
}

func (s *SignalingService) broadcastToRoom(roomID, senderID string, msg models.SignalingMessage) {
	room, exists := s.rooms[roomID]
	if !exists {
		return
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for id, client := range room {
		if id != senderID {
			select {
			case client.SendChan <- raw:
			default:
			}
		}
	}
}
