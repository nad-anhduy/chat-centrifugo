package ginchat

import (
	"net/http"
	"strings"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

// PresenceHandler handles HTTP requests for user presence operations.
type PresenceHandler struct {
	chatBiz *business.ChatBusiness
}

// NewPresenceHandler creates a new PresenceHandler.
func NewPresenceHandler(chatBiz *business.ChatBusiness) *PresenceHandler {
	return &PresenceHandler{chatBiz: chatBiz}
}

// GetBulkPresence returns the presence status for a list of user IDs.
// GET /api/v1/users/presence?ids=uuid1,uuid2,uuid3
func (h *PresenceHandler) GetBulkPresence(c *gin.Context) {
	idsParam := c.Query("ids")
	if idsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids query parameter is required"})
		return
	}

	userIDs := strings.Split(idsParam, ",")

	// Filter out empty strings from trailing commas.
	filtered := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}

	if len(filtered) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one user ID is required"})
		return
	}

	presences, err := h.chatBiz.GetBulkPresence(c.Request.Context(), filtered)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": presences})
}

// Heartbeat refreshes the authenticated user's online presence TTL.
// POST /api/v1/users/heartbeat
func (h *PresenceHandler) Heartbeat(c *gin.Context) {
	userID := c.GetString("user_id")

	if err := h.chatBiz.RefreshPresence(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "heartbeat received"})
}
