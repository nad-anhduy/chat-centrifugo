package model

import "time"

type User struct {
	ID           string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Username     string    `gorm:"type:varchar(255);uniqueIndex:uni_users_username;not null" json:"username"`
	Email        string    `gorm:"type:varchar(255)" json:"email,omitempty"`
	PasswordHash string    `gorm:"not null" json:"-"`
	PublicKey    string    `gorm:"column:public_key;type:text;not null" json:"public_key"`
	PrivateKey   string    `gorm:"column:private_key;type:text" json:"-"` // AES-GCM encrypted PKCS#8 PEM (server-side)
	AvatarURL    string    `gorm:"column:avatar_url;type:text" json:"avatar_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
