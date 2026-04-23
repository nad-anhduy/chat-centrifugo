package model

import "time"

type GroupRole string

const (
	GroupRoleCreator GroupRole = "creator"
	GroupRoleAdmin   GroupRole = "admin"
	GroupRoleMember  GroupRole = "member"
)

type GroupMember struct {
	GroupID           string    `gorm:"type:uuid;primaryKey" json:"group_id"`
	UserID            string    `gorm:"type:uuid;primaryKey;index" json:"user_id"`
	EncryptedGroupKey string    `gorm:"type:text;not null" json:"encrypted_group_key"`
	Role              GroupRole `gorm:"type:varchar(32);not null;default:'member'" json:"role"`
	JoinedAt          time.Time `json:"joined_at"`
}

func (GroupMember) TableName() string { return "group_members" }
