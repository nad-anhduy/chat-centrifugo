package ginchat

import (
	"log"
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

// WebhookHandler handles Centrifugo proxy webhook requests
// for connect/disconnect events.
type WebhookHandler struct {
	chatBiz *business.ChatBusiness
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(chatBiz *business.ChatBusiness) *WebhookHandler {
	return &WebhookHandler{chatBiz: chatBiz}
}

// centrifugoConnectRequest is the expected payload from Centrifugo's connect proxy.
type centrifugoConnectRequest struct {
	Client    string `json:"client"`
	Transport string `json:"transport"`
	Protocol  string `json:"protocol"`
	Encoding  string `json:"encoding"`
	Data      struct {
		UserID string `json:"user_id"`
	} `json:"data"`
	// User is set by Centrifugo from the JWT "sub" claim.
	User string `json:"user"`
}

// centrifugoDisconnectRequest is the expected payload from Centrifugo's disconnect proxy.
type centrifugoDisconnectRequest struct {
	Client string `json:"client"`
	User   string `json:"user"`
	Reason string `json:"reason"`
}

// OnConnect handles Centrifugo connect proxy webhook.
// It marks the user as online when they establish a WebSocket connection.
// POST /api/v1/centrifugo/connect
func (h *WebhookHandler) OnConnect(c *gin.Context) {
	var req centrifugoConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[WARN] Centrifugo connect webhook: invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	userID := req.User
	if userID == "" {
		// Fallback to data.user_id if user field is not set.
		userID = req.Data.UserID
	}

	if userID == "" {
		log.Printf("[WARN] Centrifugo connect webhook: no user ID found in request")
		// Return success to not block the connection — presence will be handled by heartbeat.
		c.JSON(http.StatusOK, gin.H{"result": gin.H{}})
		return
	}

	if err := h.chatBiz.SetUserOnline(c.Request.Context(), userID); err != nil {
		log.Printf("[ERROR] Centrifugo connect webhook: failed to set user %s online: %v", userID, err)
	}

	// Centrifugo expects a result object in the response.
	// Explicitly returning the "user" field ensures Centrifugo maps the connection correctly.
	c.JSON(http.StatusOK, gin.H{
		"result": gin.H{
			"user": userID,
		},
	})
}

// OnDisconnect handles Centrifugo disconnect proxy webhook.
// It marks the user as offline when they close their WebSocket connection.
// POST /api/v1/centrifugo/disconnect
func (h *WebhookHandler) OnDisconnect(c *gin.Context) {
	var req centrifugoDisconnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[WARN] Centrifugo disconnect webhook: invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	userID := req.User
	if userID == "" {
		log.Printf("[WARN] Centrifugo disconnect webhook: no user ID found in request")
		c.JSON(http.StatusOK, gin.H{"result": gin.H{}})
		return
	}

	if err := h.chatBiz.SetUserOffline(c.Request.Context(), userID); err != nil {
		log.Printf("[ERROR] Centrifugo disconnect webhook: failed to set user %s offline: %v", userID, err)
	}

	c.JSON(http.StatusOK, gin.H{"result": gin.H{}})
}
