package ginchat

import (
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

// UserHandler handles HTTP requests for user-related operations.
type UserHandler struct {
	chatBiz *business.ChatBusiness
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(chatBiz *business.ChatBusiness) *UserHandler {
	return &UserHandler{chatBiz: chatBiz}
}

// GetPublicKey returns the public key for a user by their UUID.
// GET /api/v1/users/:id/public-key
func (h *UserHandler) GetPublicKey(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}

	publicKey, err := h.chatBiz.GetUserPublicKey(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"public_key": publicKey}})
}

// updatePublicKeyReq is the request body for updating a user's public key.
type updatePublicKeyReq struct {
	PublicKey string `json:"public_key" binding:"required"`
}

// UpdatePublicKey updates the authenticated user's public key.
// PUT /api/v1/users/me/public-key
func (h *UserHandler) UpdatePublicKey(c *gin.Context) {
	var req updatePublicKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	if err := h.chatBiz.UpdateUserPublicKey(c.Request.Context(), userID, req.PublicKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "public key updated successfully"})
}
