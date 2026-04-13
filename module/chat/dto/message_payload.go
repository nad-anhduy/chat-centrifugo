package dto

import "time"

// CentrifugoMessagePayload is the real-time payload sent to clients via Centrifugo.
// It contains pre-resolved fields so the client can render the message
// without extra API round-trips (e.g., resolving sender_id → sender_name).
type CentrifugoMessagePayload struct {
	MessageID        string    `json:"message_id"`
	ConversationID   string    `json:"conversation_id"`
	SenderID         string    `json:"sender_id"`
	SenderName       string    `json:"sender_name"`
	ContentEncrypted string    `json:"content_encrypted"`
	CreatedAt        time.Time `json:"created_at"`
}
