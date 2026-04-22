package model

import "time"

const UserDeviceStatusActive = "active"

// UserDevice tracks client devices seen at login.
type UserDevice struct {
	ID                string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID            string    `gorm:"type:uuid;not null;uniqueIndex:uni_user_devices_user_fingerprint" json:"user_id"`
	DeviceFingerprint string    `gorm:"type:varchar(128);not null;uniqueIndex:uni_user_devices_user_fingerprint" json:"device_fingerprint"`
	UserAgent         string    `gorm:"type:text" json:"user_agent"`
	IP                string    `gorm:"type:varchar(64)" json:"ip"`
	LastLogin         time.Time `json:"last_login"`
	Status            string    `gorm:"type:varchar(32);not null;default:'active'" json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

func (UserDevice) TableName() string { return "user_devices" }

// UserDeviceChanged is an audit log when a new active device fingerprint appears.
type UserDeviceChanged struct {
	ID        string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;index" json:"user_id"`
	OldInfo   string    `gorm:"type:text" json:"old_info"`
	NewInfo   string    `gorm:"type:text;not null" json:"new_info"`
	ChangedAt time.Time `json:"changed_at"`
}

func (UserDeviceChanged) TableName() string { return "user_device_changed" }
