package model

import "time"

const (
	FriendshipStatusPending  = "pending"
	FriendshipStatusAccepted = "accepted"
	FriendshipStatusRejected = "rejected"
)

type Friendship struct {
	ID          string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	RequesterID string    `gorm:"type:uuid;not null;index" json:"requester_id"`
	ReceiverID  string    `gorm:"type:uuid;not null;index" json:"receiver_id"`
	Status      string    `gorm:"type:varchar(50);not null;default:'pending'" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Requester *User `gorm:"foreignKey:RequesterID" json:"requester,omitempty"`
	Receiver  *User `gorm:"foreignKey:ReceiverID" json:"receiver,omitempty"`
}

func (Friendship) TableName() string {
	return "friendships"
}
