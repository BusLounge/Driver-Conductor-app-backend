package models

import "encoding/json"

// SignalingMessageType defines types of WebRTC signaling payloads
type SignalingMessageType string

const (
	MessageTypeJoin         SignalingMessageType = "join"
	MessageTypeLeave        SignalingMessageType = "leave"
	MessageTypeCallRequest  SignalingMessageType = "call_request"
	MessageTypeCallAccept   SignalingMessageType = "call_accept"
	MessageTypeCallReject   SignalingMessageType = "call_reject"
	MessageTypeCallEnd      SignalingMessageType = "call_end"
	MessageTypeOffer        SignalingMessageType = "offer"
	MessageTypeAnswer       SignalingMessageType = "answer"
	MessageTypeIceCandidate SignalingMessageType = "ice_candidate"
	MessageTypeError        SignalingMessageType = "error"
)

// SignalingMessage represents the JSON structure exchanged over WebSockets
type SignalingMessage struct {
	Type     SignalingMessageType `json:"type"`
	RoomID   string               `json:"room_id"`
	SenderID string               `json:"sender_id"`
	Role     string               `json:"role,omitempty"` // "driver", "conductor", "passenger"
	TargetID string               `json:"target_id,omitempty"`
	Payload  json.RawMessage      `json:"payload,omitempty"`
}

// CallSession represents an active room with connected peers
type CallSession struct {
	RoomID    string
	CreatedAt string
	Peers     map[string]string // map[senderID]role
}
