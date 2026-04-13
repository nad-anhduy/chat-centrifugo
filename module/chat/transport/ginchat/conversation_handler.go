package ginchat

import (
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

// ConversationHandler handles HTTP requests for conversation management.
type ConversationHandler struct {
	chatBiz *business.ChatBusiness
}

// NewConversationHandler creates a new ConversationHandler.
func NewConversationHandler(chatBiz *business.ChatBusiness) *ConversationHandler {
	return &ConversationHandler{chatBiz: chatBiz}
}

// ListConversations returns all conversations for the authenticated user.
// GET /api/v1/conversations
func (h *ConversationHandler) ListConversations(c *gin.Context) {
	userID := c.GetString("user_id")

	conversations, err := h.chatBiz.GetConversationsByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": conversations})
}

// CreateGroup creates a new group conversation with the specified members.
// POST /api/v1/conversations
func (h *ConversationHandler) CreateGroup(c *gin.Context) {
	var req business.CreateGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	conv, err := h.chatBiz.CreateGroupConversation(c.Request.Context(), userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": conv})
}
