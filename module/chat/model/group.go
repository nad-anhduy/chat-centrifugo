package model

import "time"

type Group struct {
	ID        string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	Avatar    string    `gorm:"type:text" json:"avatar,omitempty"`
	CreatorID string    `gorm:"type:uuid;not null;index" json:"creator_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Group) TableName() string { return "groups" }
