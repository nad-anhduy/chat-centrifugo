package ginchat

import (
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

// UserHandler handles HTTP requests for user-related operations.
type UserHandler struct {
	chatBiz   *business.ChatBusiness
	friendBiz *business.FriendshipBusiness // For tasks like SearchUsers
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(chatBiz *business.ChatBusiness, friendBiz *business.FriendshipBusiness) *UserHandler {
	return &UserHandler{chatBiz: chatBiz, friendBiz: friendBiz}
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

// SearchUsers finds users matching a query.
// GET /api/v1/users/search?q=...
func (h *UserHandler) SearchUsers(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	userID := c.GetString("user_id")

	users, err := h.friendBiz.SearchUsers(c.Request.Context(), query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Format response to include only necessary fields: id, username, email, public_key
	type userRes struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		PublicKey string `json:"public_key"`
	}

	res := make([]userRes, len(users))
	for i, u := range users {
		res[i] = userRes{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			PublicKey: u.PublicKey,
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": res})
}
