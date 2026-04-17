package model

import "time"

// PresenceStatus represents the online/offline state of a user.
type PresenceStatus string

const (
	PresenceStatusOnline  PresenceStatus = "online"
	PresenceStatusOffline PresenceStatus = "offline"
)

// PresenceInfo holds the presence state for a single user.
type PresenceInfo struct {
	UserID   string         `json:"user_id"`
	Status   PresenceStatus `json:"status"`
	LastSeen time.Time      `json:"last_seen"`
}
