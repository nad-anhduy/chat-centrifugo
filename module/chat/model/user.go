package model

import "time"

type User struct {
	ID           string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Username     string    `gorm:"type:varchar(255);uniqueIndex:uni_users_username;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	PublicKey    string    `gorm:"column:public_key;type:text;not null" json:"public_key"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
