package model

import "time"

type Participant struct {
	ConversationID string    `gorm:"type:uuid;primary_key" json:"conversation_id"`
	UserID         string    `gorm:"type:uuid;primary_key" json:"user_id"`
	JoinedAt       time.Time `json:"joined_at"`
}

func (Participant) TableName() string {
	return "participants"
}
