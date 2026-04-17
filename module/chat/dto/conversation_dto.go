package dto

import (
	"be-chat-centrifugo/module/chat/model"
	"time"
)

// ParticipantDetail contains selected user information for conversation members.
type ParticipantDetail struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	PublicKey string `json:"public_key"`
}

// ConversationDetail provides a comprehensive view of a conversation including its participants.
type ConversationDetail struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         model.ConversationType `json:"type"`
	Participants []ParticipantDetail    `json:"participants"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}
