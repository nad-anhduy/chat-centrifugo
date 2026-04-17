package dto

import (
	"time"

	"be-chat-centrifugo/module/chat/model"
)

// PresenceUpdate is the real-time payload published via Centrifugo
// when a user's presence status changes.
type PresenceUpdate struct {
	Type     string               `json:"type"` // always "presence_update"
	UserID   string               `json:"user_id"`
	Status   model.PresenceStatus `json:"status"`
	LastSeen time.Time            `json:"last_seen"`
}
