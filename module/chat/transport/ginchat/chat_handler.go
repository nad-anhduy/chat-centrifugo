package ginchat

import (
	"net/http"

	"be-chat-centrifugo/module/chat/business"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	chatBiz *business.ChatBusiness
}

func NewChatHandler(chatBiz *business.ChatBusiness) *ChatHandler {
	return &ChatHandler{chatBiz: chatBiz}
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	var req business.SendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	if err := h.chatBiz.SendMessage(c.Request.Context(), userID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "message sent"})
}
