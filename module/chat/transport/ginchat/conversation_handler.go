package ginchat

import (
	"net/http"
	"strconv"
	"time"

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

// GetMessages returns paginated messages for a conversation.
// GET /api/v1/conversations/:id/messages?limit=100&before_ts=1713340800000
//
// Query parameters:
//   - limit: number of messages to return (default: 100, max: 500)
//   - before_ts: Unix timestamp in milliseconds for cursor-based pagination.
//     Omit for the first page (latest messages).
func (h *ConversationHandler) GetMessages(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
		return
	}

	// Parse limit with default/bounds.
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit parameter"})
			return
		}
		limit = parsed
	}

	// Parse before_ts (Unix milliseconds) for cursor-based pagination.
	var beforeTS time.Time
	if tsStr := c.Query("before_ts"); tsStr != "" {
		tsMillis, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_ts parameter, expected Unix milliseconds"})
			return
		}
		beforeTS = time.UnixMilli(tsMillis)
	}

	messages, err := h.chatBiz.GetMessages(c.Request.Context(), conversationID, beforeTS, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  messages,
		"count": len(messages),
	})
}
