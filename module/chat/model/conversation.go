package model

import "time"

type ConversationType string

const (
	ConversationTypeDirect ConversationType = "DIRECT"
	ConversationTypeGroup  ConversationType = "GROUP"
)

type Conversation struct {
	ID        string           `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name      string           `gorm:"type:varchar(255)" json:"name"`
	Type      ConversationType `gorm:"type:varchar(50);not null" json:"type"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func (Conversation) TableName() string {
	return "conversations"
}
