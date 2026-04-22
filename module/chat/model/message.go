package model

import "time"

type Message struct {
	ConversationID   string    `json:"conversation_id"` // Partition Key (UUID)
	CreatedAt        time.Time `json:"created_at"`      // Clustering Key (Timestamp, desc)
	MessageID        string    `json:"message_id"`      // Clustering Key (TimeUUID)
	SenderID         string    `json:"sender_id"`
	ContentEncrypted string    `json:"content_encrypted"`
	KeyForSender     string    `json:"key_for_sender,omitempty"`
	KeyForReceiver   string    `json:"key_for_receiver,omitempty"`
	IV               string    `json:"iv,omitempty"`
	IsRead           bool      `json:"is_read"`
}
